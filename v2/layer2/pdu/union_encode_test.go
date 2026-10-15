package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
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
				_, err := pdu.EncodeFullLinkControl(&pdu.FullLinkControl{
					DataType:   elements.DataTypeVoiceLCHeader,
					FLCO:       enums.FLCOUnitToUnitVoiceChannelUser,
					GroupVoice: &pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 1234},
				})
				return err
			},
			want: "FullLinkControl.GroupVoice is set, but FLCO and FeatureSetID select UnitToUnit",
		},
		{
			name: "FLC manufacturer FID",
			encode: func() error {
				_, err := pdu.EncodeFullLinkControl(&pdu.FullLinkControl{
					DataType:     elements.DataTypeVoiceLCHeader,
					FLCO:         enums.FLCOGroupVoiceChannelUser,
					FeatureSetID: enums.MotorolaLtd,
					GroupVoice:   &pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 1234},
				})
				return err
			},
			want: "FullLinkControl.GroupVoice is set, but FLCO and FeatureSetID select RawPayload",
		},
		{
			name: "two FLC payloads",
			encode: func() error {
				_, err := pdu.EncodeFullLinkControl(&pdu.FullLinkControl{
					DataType:   elements.DataTypeVoiceLCHeader,
					FLCO:       enums.FLCOUnitToUnitVoiceChannelUser,
					GroupVoice: &pdu.FLCGroupVoice{},
					UnitToUnit: &pdu.FLCUnitToUnit{},
				})
				return err
			},
			want: "FullLinkControl: GroupVoice and UnitToUnit are both set",
		},
		{
			name: "CSBKO",
			encode: func() error {
				_, err := pdu.EncodeCSBK(&pdu.CSBK{
					CSBKOpcode:             pdu.CSBKAloha,
					TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{},
				})
				return err
			},
			want: "CSBK.TalkgroupVoiceGrantPDU is set, but CSBKOpcode, FID and TrunkingMode select AlohaPDU",
		},
		{
			name: "CSBK manufacturer FID",
			encode: func() error {
				_, err := pdu.EncodeCSBK(&pdu.CSBK{
					CSBKOpcode:             pdu.CSBKTalkgroupVoiceGrant,
					FID:                    0x10,
					TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{},
				})
				return err
			},
			want: "CSBK.TalkgroupVoiceGrantPDU is set, but CSBKOpcode, FID and TrunkingMode select RawPayload",
		},
		{
			name: "CSBK opcode 0x38 outside trunking",
			encode: func() error {
				_, err := pdu.EncodeCSBK(&pdu.CSBK{
					CSBKOpcode:                     pdu.CSBKBSOutboundActivationPDU,
					TalkgroupDataGrantMultiItemPDU: &pdu.TalkgroupDataGrantMultiItemPDU{},
				})
				return err
			},
			want: "select BSOutboundActivationPDU",
		},
		{
			name: "Format",
			encode: func() error {
				_, err := pdu.EncodeDataHeader(&pdu.DataHeader{
					Format:                pdu.FormatConfirmed,
					UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{},
				})
				return err
			},
			want: "DataHeader.UnconfirmedDataHeader is set, but Format and AppendedBlocks select ConfirmedDataHeader",
		},
		{
			name: "R_HEAD without appended blocks",
			encode: func() error {
				_, err := pdu.EncodeDataHeader(&pdu.DataHeader{
					Format:        pdu.FormatShortDataRawOrStatusPrecoded,
					RawDataHeader: &pdu.RawDataHeader{},
				})
				return err
			},
			want: "select StatusPrecodedHeader",
		},
		{
			name: "SLCO",
			encode: func() error {
				_, err := pdu.EncodeShortLC(&pdu.ShortLC{
					SLCO:           enums.SLCONullMessage,
					ActivityUpdate: &pdu.ShortLCActivityUpdate{HashTS1: 0xAB},
				})
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
		CSBKOpcode:                     pdu.CSBKBSOutboundActivationPDU,
		TrunkingMode:                   true,
		TalkgroupDataGrantMultiItemPDU: &pdu.TalkgroupDataGrantMultiItemPDU{},
	}
	got, _ := pdu.DecodeCSBK(must(pdu.EncodeCSBK(&csbk)), true)
	if got.TalkgroupDataGrantMultiItemPDU == nil {
		t.Error("opcode 0x38 in trunking mode did not decode as TalkgroupDataGrantMultiItemPDU")
	}

	dh := pdu.DataHeader{
		Format:        pdu.FormatShortDataRawOrStatusPrecoded,
		RawDataHeader: &pdu.RawDataHeader{AppendedBlocks: 3},
	}
	gotDH, _ := pdu.DecodeDataHeader(must(pdu.EncodeDataHeader(&dh)))
	if gotDH.RawDataHeader == nil || gotDH.RawDataHeader.AppendedBlocks != 3 {
		t.Errorf("R_HEAD round trip = %+v", gotDH.RawDataHeader)
	}

	lc := pdu.FullLinkControl{
		DataType:         elements.DataTypeTerminatorWithLC,
		FLCO:             enums.FLCOTalkerAliasBlock2,
		TalkerAliasBlock: &pdu.FLCTalkerAliasBlock{},
	}
	if _, err := pdu.EncodeFullLinkControl(&lc); err != nil {
		t.Errorf("Talker Alias block 2: %v", err)
	}
}
