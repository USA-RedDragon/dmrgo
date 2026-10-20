package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestEncodeRejectsPayloadTheHeaderDoesNotSelect(t *testing.T) {
	t.Parallel()

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
			want: "FullLinkControl: the payload field that is set is not the one that FLCO and FID select",
		},
		{
			name: "FLC manufacturer FID",
			encode: func() error {
				_, err := (&pdu.FullLinkControl{
					BurstDataType: enums.DataTypeVoiceLCHeader,
					FLCO:          enums.FLCOGroupVoiceChannelUser,
					FID:           enums.FIDMotorola,
					GroupVoice:    &pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 1234},
				}).Encode()
				return err
			},
			want: "FullLinkControl: the payload field that is set is not the one that FLCO and FID select",
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
			want: "FullLinkControl: more than one payload field is set",
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
			want: "CSBK: the payload field that is set is not the one that Opcode and FID select",
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
			want: "CSBK: the payload field that is set is not the one that Opcode and FID select",
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
			want: "CSBK: the payload field that is set is not the one that Opcode and FID select",
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
			want: "DataHeader: the payload field that is set is not the one that Format and AppendedBlocks select",
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
			want: "DataHeader: the payload field that is set is not the one that Format and AppendedBlocks select",
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
			want: "ShortLC: the payload field that is set is not the one that SLCO selects",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			expectEncodeError(t, tt.encode(), tt.want)
		})
	}
}

func TestEncodeAcceptsMatchingPayload(t *testing.T) {
	t.Parallel()

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
