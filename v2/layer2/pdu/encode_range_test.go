package pdu_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
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
	t.Parallel()

	_, err := (&pdu.FLCGroupVoice{GroupAddress: 1 << 24}).Encode()
	expectEncodeError(t, err, "FLCGroupVoice.GroupAddress: value 16777216 does not fit in 24 bits")

	var rangeErr *bit.FieldRangeError
	if !errors.As(err, &rangeErr) || rangeErr.Type != "FLCGroupVoice" || rangeErr.Field != "GroupAddress" || rangeErr.Bits != 24 || rangeErr.Value != uint32(1<<24) {
		t.Fatalf("error %#v is not the FieldRangeError of GroupAddress", err)
	}

	_, err = (&pdu.FLCGroupVoice{SourceAddress: 0xFFFFFFFF}).Encode()
	expectEncodeError(t, err, "FLCGroupVoice.SourceAddress: value 4294967295 does not fit in 24 bits")

	_, err = (&pdu.SlotType{ColorCode: 16}).Encode()
	expectEncodeError(t, err, "SlotType.ColorCode")

	_, err = (&pdu.SlotType{DataType: enums.DataType(16)}).Encode()
	expectEncodeError(t, err, "SlotType.DataType")

	_, err = (&pdu.UnconfirmedDataHeader{PadOctetCount: 32}).Encode()
	expectEncodeError(t, err, "UnconfirmedDataHeader.PadOctetCount: value 32 does not fit in 5 bits")

	_, err = (&pdu.FLCTalkerAliasHeader{TalkerAliasDataLength: 32}).Encode()
	expectEncodeError(t, err, "TalkerAliasDataLength: value 32 does not fit in 5 bits")
}

func TestEncode_AcceptsMaximumFieldValues(t *testing.T) {
	t.Parallel()

	if _, err := (&pdu.FLCGroupVoice{GroupAddress: 1<<24 - 1, SourceAddress: 1<<24 - 1}).Encode(); err != nil {
		t.Fatalf("max addresses rejected: %v", err)
	}
	if _, err := (&pdu.SlotType{ColorCode: 15, DataType: enums.DataTypeReserved}).Encode(); err != nil {
		t.Fatalf("max slot type rejected: %v", err)
	}
	if _, err := (&pdu.UnconfirmedDataHeader{PadOctetCount: 31}).Encode(); err != nil {
		t.Fatalf("max pad octet count rejected: %v", err)
	}
}

func TestEncode_PropagatesNestedErrors(t *testing.T) {
	t.Parallel()

	_, err := (&pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		GroupVoice:    &pdu.FLCGroupVoice{GroupAddress: 1 << 24},
	}).Encode()
	expectEncodeError(t, err, "FullLinkControl.GroupVoice: FLCGroupVoice.GroupAddress")

	_, err = (&pdu.FullLinkControl{FLCO: enums.FLCO(64)}).Encode()
	expectEncodeError(t, err, "FullLinkControl.FLCO: value 64 does not fit in 6 bits")

	_, err = (&pdu.CSBK{Opcode: 64}).Encode()
	expectEncodeError(t, err, "CSBK.Opcode: value 64 does not fit in 6 bits")
}

func TestEncode_RejectsCoordinatesOutOfRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		lon, lat float64
		want     string
	}{
		{"LatitudeAbove", 0, 90.0001, "Latitude: value 90.0001 is outside [-90, 90]"},
		{"LatitudeBelow", 0, -90.0001, "Latitude"},
		{"LongitudeAbove", 180.0001, 0, "Longitude: value 180.0001 is outside [-180, 180]"},
		{"LongitudeBelow", -180.0001, 0, "Longitude"},
		{"NaN", math.NaN(), 0, "Longitude"},
		{"Inf", 0, math.Inf(1), "Latitude"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := (&pdu.FLCGPSInfo{Longitude: tt.lon, Latitude: tt.lat}).Encode()
			expectEncodeError(t, err, tt.want)
		})
	}
}
