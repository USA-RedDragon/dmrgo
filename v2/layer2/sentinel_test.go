package layer2_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func TestMisuseSentinels(t *testing.T) {
	t.Parallel()

	var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	_, voiceErr := layer2.BuildVoiceBurst(frames, enums.VoiceBurstUnknown, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSourceBS)
	_, voiceSrcErr := layer2.BuildVoiceBurst(frames, enums.VoiceBurstA, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSource(9))
	_, notBPTC := layer2.BuildDataBurst([12]byte{}, enums.DataTypeRate34, 0, layer2.SyncSourceBS)
	_, dataSrcErr := layer2.BuildDataBurst([12]byte{}, enums.DataTypeCSBK, 0, layer2.SyncSource(9))
	_, rate1SrcErr := layer2.BuildRate1DataBurst([24]byte{}, 0, layer2.SyncSource(9))
	_, txErr := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{Source: layer2.SyncSource(9)})
	b := layer2.Burst{
		Kind: layer2.KindData, SyncPattern: enums.SyncBSSourcedData,
		SlotType: pdu.SlotType{DataType: enums.DataTypeCSBK}, Data: &pdu.FullLinkControl{},
	}
	_, encodeErr := b.Encode()
	for _, c := range []struct {
		name string
		err  error
		want error
	}{
		{"BuildVoiceBurst position", voiceErr, layer2.ErrVoicePosition},
		{"BuildVoiceBurst source", voiceSrcErr, layer2.ErrSyncSource},
		{"BuildDataBurst rate 3/4", notBPTC, layer2.ErrDataType},
		{"BuildDataBurst source", dataSrcErr, layer2.ErrSyncSource},
		{"BuildRate1DataBurst source", rate1SrcErr, layer2.ErrSyncSource},
		{"NewVoiceTransmitter source", txErr, layer2.ErrSyncSource},
		{"Encode data type", encodeErr, layer2.ErrDataType},
	} {
		if !errors.Is(c.err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, c.err, c.want)
		}
	}
}
