package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// The receiver selects the payload variant from the discriminator, so Encode
// must reject a payload pointer that the encoded header would not select.
func TestEncodeRejectsPayloadTheHeaderDoesNotSelect(t *testing.T) {
	tests := []struct {
		name   string
		encode func() error
		want   string
	}{
		{
			name: "FLCO",
			encode: func() error {
				_, err := (&pdu.FullLinkControl{
					BurstDataType: enums.DataTypeVoiceLCHeader,
					FLCO:          enums.FLCOUnitToUnitVoiceChannelUser,
					GroupVoice:    &pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 1234},
				}).Encode()
				return err
			},
			want: "FullLinkControl.GroupVoice is set, but FLCO and FeatureSetID select UnitToUnit",
		},
		{
			name: "FLC manufacturer FID",
			encode: func() error {
				_, err := (&pdu.FullLinkControl{
					BurstDataType: enums.DataTypeVoiceLCHeader,
					FLCO:          enums.FLCOGroupVoiceChannelUser,
					FeatureSetID:  enums.FIDMotorola,
					GroupVoice:    &pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 1234},
				}).Encode()
				return err
			},
			want: "FullLinkControl.GroupVoice is set, but FLCO and FeatureSetID select RawPayload",
		},
		{
			name: "two FLC payloads",
			encode: func() error {
				_, err := (&pdu.FullLinkControl{
					BurstDataType: enums.DataTypeVoiceLCHeader,
					FLCO:          enums.FLCOUnitToUnitVoiceChannelUser,
					GroupVoice:    &pdu.FLCGroupVoice{},
					UnitToUnit:    &pdu.FLCUnitToUnit{},
				}).Encode()
				return err
			},
			want: "FullLinkControl: GroupVoice and UnitToUnit are both set",
		},
		{
			name: "CSBKO",
			encode: func() error {
				_, err := (&pdu.CSBK{
					Opcode:                 pdu.CSBKAloha,
					TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{},
				}).Encode()
				return err
			},
			want: "CSBK.TalkgroupVoiceGrantPDU is set, but Opcode and FID select AlohaPDU",
		},
		{
			name: "CSBK manufacturer FID",
			encode: func() error {
				_, err := (&pdu.CSBK{
					Opcode:                 pdu.CSBKTalkgroupVoiceGrant,
					FID:                    0x10,
					TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{},
				}).Encode()
				return err
			},
			want: "CSBK.TalkgroupVoiceGrantPDU is set, but Opcode and FID select RawPayload",
		},
		{
			name: "CSBK opcode 0x38",
			encode: func() error {
				_, err := (&pdu.CSBK{
					Opcode:                 pdu.CSBKTalkgroupDataGrantMultiItem,
					TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{},
				}).Encode()
				return err
			},
			want: "Opcode and FID select RawPayload",
		},
		{
			name: "Format",
			encode: func() error {
				_, err := (&pdu.DataHeader{
					Format:                pdu.FormatConfirmed,
					UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{},
				}).Encode()
				return err
			},
			want: "DataHeader.UnconfirmedDataHeader is set, but Format and AppendedBlocks select ConfirmedDataHeader",
		},
		{
			name: "R_HEAD without appended blocks",
			encode: func() error {
				_, err := (&pdu.DataHeader{
					Format:        pdu.FormatShortDataRawOrStatusPrecoded,
					RawDataHeader: &pdu.RawDataHeader{},
				}).Encode()
				return err
			},
			want: "select StatusPrecodedHeader",
		},
		{
			name: "SLCO",
			encode: func() error {
				_, err := (&pdu.ShortLC{
					SLCO:           enums.SLCONullMessage,
					ActivityUpdate: &pdu.ShortLCActivityUpdate{HashTS1: 0xAB},
				}).Encode()
				return err
			},
			want: "ShortLC.ActivityUpdate is set, but SLCO selects NullMessage",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectEncodeError(t, tt.encode(), tt.want)
		})
	}
}

func TestEncodeAcceptsMatchingPayload(t *testing.T) {
	csbk := pdu.CSBK{
		Opcode:                 pdu.CSBKTalkgroupVoiceGrant,
		TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{},
	}
	got := pdu.DecodeCSBK(must(csbk.Encode()))
	if got.TalkgroupVoiceGrantPDU == nil {
		t.Error("TV_GRANT did not decode as TalkgroupVoiceGrantPDU")
	}

	dh := pdu.DataHeader{
		Format:        pdu.FormatShortDataRawOrStatusPrecoded,
		RawDataHeader: &pdu.RawDataHeader{AppendedBlocks: 3},
	}
	gotDH := pdu.DecodeDataHeader(must(dh.Encode()))
	if gotDH.RawDataHeader == nil || gotDH.RawDataHeader.AppendedBlocks != 3 {
		t.Errorf("R_HEAD round trip = %+v", gotDH.RawDataHeader)
	}

	lc := pdu.FullLinkControl{
		BurstDataType:    enums.DataTypeTerminatorWithLC,
		FLCO:             enums.FLCOTalkerAliasBlock2,
		TalkerAliasBlock: &pdu.FLCTalkerAliasBlock{},
	}
	if _, err := lc.Encode(); err != nil {
		t.Errorf("Talker Alias block 2: %v", err)
	}
}
