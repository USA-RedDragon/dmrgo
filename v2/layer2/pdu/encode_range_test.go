package pdu_test

import (
	"math"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func expectEncodeError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}
}

func TestEncode_RejectsValuesWiderThanField(t *testing.T) {
	_, err := pdu.EncodeFLCGroupVoice(&pdu.FLCGroupVoice{GroupAddress: 1 << 24})
	expectEncodeError(t, err, "FLCGroupVoice.GroupAddress: value 16777216 does not fit in 24 bits")

	_, err = pdu.EncodeFLCGroupVoice(&pdu.FLCGroupVoice{SourceAddress: -1})
	expectEncodeError(t, err, "FLCGroupVoice.SourceAddress: value -1 does not fit in 24 bits")

	_, err = pdu.EncodeSlotType(&pdu.SlotType{ColorCode: 16})
	expectEncodeError(t, err, "SlotType.ColorCode")

	_, err = pdu.EncodeSlotType(&pdu.SlotType{DataType: elements.DataType(16)})
	expectEncodeError(t, err, "SlotType.DataType")

	_, err = pdu.EncodeUnconfirmedDataHeader(&pdu.UnconfirmedDataHeader{PadOctetCount: 32})
	expectEncodeError(t, err, "UnconfirmedDataHeader.PadOctetCount: value 32 does not fit in 5 bits")

	_, err = pdu.EncodeFLCTalkerAliasHeader(&pdu.FLCTalkerAliasHeader{TalkerAliasDataLength: 32})
	expectEncodeError(t, err, "TalkerAliasDataLength: value 32 does not fit in 5 bits")
}

func TestEncode_AcceptsMaximumFieldValues(t *testing.T) {
	if _, err := pdu.EncodeFLCGroupVoice(&pdu.FLCGroupVoice{GroupAddress: 1<<24 - 1, SourceAddress: 1<<24 - 1}); err != nil {
		t.Fatalf("max addresses rejected: %v", err)
	}
	if _, err := pdu.EncodeSlotType(&pdu.SlotType{ColorCode: 15, DataType: elements.DataTypeReserved}); err != nil {
		t.Fatalf("max slot type rejected: %v", err)
	}
	if _, err := pdu.EncodeUnconfirmedDataHeader(&pdu.UnconfirmedDataHeader{PadOctetCount: 31}); err != nil {
		t.Fatalf("max pad octet count rejected: %v", err)
	}
}

func TestEncode_PropagatesNestedErrors(t *testing.T) {
	_, err := pdu.EncodeFullLinkControl(&pdu.FullLinkControl{
		FLCO:       enums.FLCOGroupVoiceChannelUser,
		GroupVoice: &pdu.FLCGroupVoice{GroupAddress: 1 << 24},
	})
	expectEncodeError(t, err, "FullLinkControl.GroupVoice: FLCGroupVoice.GroupAddress")

	_, err = pdu.EncodeFullLinkControl(&pdu.FullLinkControl{FLCO: enums.FLCO(64)})
	expectEncodeError(t, err, "FullLinkControl.FLCO: value 64 does not fit in 6 bits")

	_, err = pdu.EncodeCSBK(&pdu.CSBK{CSBKOpcode: 64})
	expectEncodeError(t, err, "CSBK.CSBKOpcode: value 64 does not fit in 6 bits")
}

func TestEncode_RejectsCoordinatesOutOfRange(t *testing.T) {
	tests := []struct {
		name     string
		lon, lat float64
		want     string
	}{
		{"Latitude90", 0, 90, "Latitude: 90 degrees is outside [-90, 90)"},
		{"LatitudeBelow", 0, -90.0001, "Latitude"},
		{"Longitude180", 180, 0, "Longitude: 180 degrees is outside [-180, 180)"},
		{"LongitudeBelow", -180.0001, 0, "Longitude"},
		{"NaN", math.NaN(), 0, "Longitude"},
		{"Inf", 0, math.Inf(1), "Latitude"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pdu.EncodeFLCGPSInfo(&pdu.FLCGPSInfo{Longitude: tt.lon, Latitude: tt.lat})
			expectEncodeError(t, err, tt.want)
		})
	}
}
