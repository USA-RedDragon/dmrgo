package layer2_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

type notPDU struct{}

func (notPDU) String() string { return "notPDU" }

func (notPDU) DataType() enums.DataType { return enums.DataTypeCSBK }

func TestErrorsMatchSentinels(t *testing.T) {
	t.Parallel()

	raw := must(layer2.BuildDataBurst([12]byte{}, enums.DataTypeIdle, 1, layer2.SyncSourceBS))
	decoded := must(layer2.NewBurstFromBytes(raw))
	decoded.Data = nil
	decoded.SlotType.DataType = enums.DataTypeCSBK
	_, payloadType := decoded.Encode()

	var unknown layer2.Burst
	unknownKind := unknown.DecodeFromBytesWith(raw, &layer2.DecodeOptions{Kind: layer2.Kind(99)})

	var r layer2.RawBurst
	tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: groupLC(enums.FIDStandardized), ColorCode: 1, Source: layer2.SyncSourceMS, Silence: silenceSlot()}))

	encode := func(b *layer2.Burst) error {
		_, err := b.Encode()
		return err
	}
	_, notPDUErr := layer2.BuildBurst(notPDU{}, 1, layer2.SyncSourceBS)
	tests := []struct {
		name string
		err  error
		want error
	}{
		{"decode unknown kind", unknownKind, layer2.ErrKind},
		{"encode unknown kind", encode(&layer2.Burst{Kind: layer2.Kind(99)}), layer2.ErrKind},
		{"encode KindUnknown", encode(&layer2.Burst{}), layer2.ErrKind},
		{"data with voice sync", encode(&layer2.Burst{Kind: layer2.KindData, SyncPattern: enums.SyncMSSourcedVoice}), layer2.ErrSyncPattern},
		{"data without sync", encode(&layer2.Burst{Kind: layer2.KindData, SlotType: pdu.SlotType{ColorCode: 1, DataType: enums.DataTypeIdle}, Data: &pdu.PRFill{}}), layer2.ErrSyncPattern},
		{"SetSync", r.SetSync(enums.SyncNone), layer2.ErrSyncPattern},
		{"no Data", encode(&layer2.Burst{Kind: layer2.KindData, SyncPattern: enums.SyncBSSourcedData, SlotType: pdu.SlotType{ColorCode: 1, DataType: enums.DataTypeCSBK}}), layer2.ErrDataType},
		{"payload type", payloadType, layer2.ErrDataType},
		{"not a PDU", notPDUErr, layer2.ErrDataType},
		{"MS reverse channel", tx.SetReverseChannel(pdu.ReverseChannel{}), layer2.ErrSyncSource},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, tt.err, tt.want)
		}
	}
}
