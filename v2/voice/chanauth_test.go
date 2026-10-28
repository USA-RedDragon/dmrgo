package voice_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func outbound(t *testing.T, source uint32) (vh, term, idle layer2.RawBurst) {
	t.Helper()
	lc, err := pdu.NewVoiceLC(true, 9, source, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	vt, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceBS, Silence: vocoder.AMBESilenceSlot()})
	if err != nil {
		t.Fatal(err)
	}
	idle, err = layer2.BuildDataBurst(pdu.PRFillBits(), enums.DataTypeIdle, 1, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	return vt.Header(), vt.Terminator(), idle
}

func authTransmitter(t *testing.T, cfg voice.TransmitterConfig) *voice.Transmitter {
	t.Helper()
	cfg.Silence, cfg.ColorCode = vocoder.AMBESilenceSlot(), 1
	if cfg.ChanAuth == 0 {
		cfg.ChanAuth = constants.TMSChanAuth
	}
	tx := newTransmitter(t, &cfg)
	if err := tx.Start(time.Second, voice.Call{Group: true, Target: 9, Source: 3120005}, &voice.CallOptions{HangTime: true}); err != nil {
		t.Fatal(err)
	}
	if tx.State() != voice.TransmitterChanAuth {
		t.Fatalf("state %v", tx.State())
	}
	if err := tx.Frames(vocoder.AMBESilence(), vocoder.AMBESilence(), vocoder.AMBESilence()); err != nil {
		t.Fatal(err)
	}
	out := tx.Tick(time.Second, nil)
	if len(out) != 1 || out[0].Hint.DataType != enums.DataTypeVoiceLCHeader || out[0].Due != time.Second {
		t.Fatalf("ChanAuth PDU %+v", out)
	}
	if at, ok := tx.Deadline(); !ok || at != time.Second+cfg.ChanAuth {
		t.Fatalf("deadline %v", at)
	}
	return tx
}

// ETSI TS 102 361-2 clause 5.1.1.6.2
func TestTransmitter_ChanAuth(t *testing.T) {
	t.Parallel()

	own, ownTerm, idle := outbound(t, 3120005)
	other, _, _ := outbound(t, 3120006)
	ms := time.Millisecond

	tx := authTransmitter(t, voice.TransmitterConfig{Slot: enums.Slot1})
	tx.Burst(1030*ms, &idle, layer2.Hint{}, nil)
	tx.Burst(1090*ms, &own, layer2.Hint{}, nil)
	if tx.State() != voice.TransmitterHeader {
		t.Fatalf("own ChanAuth PDU: %v", tx.State())
	}
	tx.Stop()
	out := drain(t, tx, 20)
	if shape(out) != "hABCDEFt." || out[0].Due != 1090*ms {
		t.Fatalf("outputs %s, due %v", shape(out), out[0].Due)
	}

	tx = authTransmitter(t, voice.TransmitterConfig{Slot: enums.Slot1})
	out = tx.Burst(1090*ms, &other, layer2.Hint{}, nil)
	if len(out) != 1 || out[0].Kind != voice.OutputEnded || out[0].Reason != voice.EndDenied || tx.State() != voice.TransmitterIdle || tx.Room() != voice.DefaultQueue {
		t.Fatalf("other ChanAuth PDU: %+v", out)
	}

	tx = authTransmitter(t, voice.TransmitterConfig{Slot: enums.Slot1, HoldoffMax: 500 * ms, ChanAuth: 360 * ms, Seed: 4})
	tx.Burst(1030*ms, &ownTerm, layer2.Hint{}, nil)
	if out := tx.Tick(1360*ms, nil); len(out) != 0 || tx.State() != voice.TransmitterChanAuth {
		t.Fatalf("expiry in hang time: %+v %v", out, tx.State())
	}
	at, ok := tx.Deadline()
	if !ok || at < 1360*ms || at > 1860*ms {
		t.Fatalf("holdoff to %v", at)
	}
	if out := tx.Tick(at, nil); len(out) != 1 || out[0].Hint.DataType != enums.DataTypeVoiceLCHeader || out[0].Due != at {
		t.Fatalf("second ChanAuth PDU %+v", out)
	}
	if out := tx.Tick(at+360*ms, nil); len(out) != 1 || out[0].Kind != voice.OutputEnded || out[0].Reason != voice.EndNoOutbound || out[0].Due != at+360*ms || tx.State() != voice.TransmitterIdle {
		t.Fatalf("expiry without hang time: %+v %v", out, tx.State())
	}

	tx = authTransmitter(t, voice.TransmitterConfig{Slot: enums.Slot1, HoldoffMax: -1})
	tx.Burst(1030*ms, &idle, layer2.Hint{}, nil)
	tx.Tick(1180*ms, nil)
	if d, _ := tx.Deadline(); d != 1180*ms || tx.State() != voice.TransmitterChanAuth {
		t.Fatalf("no holdoff: %v", d)
	}
	tx.Stop()
	if tx.State() != voice.TransmitterIdle || tx.Room() != voice.DefaultQueue {
		t.Fatal("Stop in ChanAuth")
	}
}

// ETSI TS 102 361-2 clause 5.1.1.6.2
func TestTransmitter_ChanAuthOutbound(t *testing.T) {
	t.Parallel()

	ms := time.Millisecond
	tx := authTransmitter(t, voice.TransmitterConfig{Slot: enums.Slot1, HoldoffMax: -1})
	out := tx.Tick(1180*ms, nil)
	if len(out) != 1 || out[0].Kind != voice.OutputEnded || out[0].Reason != voice.EndNoOutbound || tx.State() != voice.TransmitterIdle || tx.Room() != voice.DefaultQueue {
		t.Fatalf("silent outbound: %+v %v", out, tx.State())
	}

	lc, err := pdu.NewVoiceLC(true, 9, 3120006, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	vt, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceBS, Silence: vocoder.AMBESilenceSlot()})
	if err != nil {
		t.Fatal(err)
	}
	voiceBursts, err := vt.AddFrames(vocoder.AMBESilence(), vocoder.AMBESilence(), vocoder.AMBESilence())
	if err != nil || len(voiceBursts) != 1 {
		t.Fatalf("voice burst: %v", err)
	}
	pi, err := layer2.BuildDataBurst(pdu.PRFillBits(), enums.DataTypePIHeader, 1, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	junk := layer2.RawBurst{}
	for name, b := range map[string]layer2.RawBurst{"voice": voiceBursts[0], "PI header": pi, "undecodable": junk} {
		tx := authTransmitter(t, voice.TransmitterConfig{Slot: enums.Slot1, HoldoffMax: -1})
		tx.Burst(1030*ms, &b, layer2.Hint{}, nil)
		if out := tx.Tick(1180*ms, nil); len(out) != 0 || tx.State() != voice.TransmitterChanAuth {
			t.Fatalf("%s: %+v %v", name, out, tx.State())
		}
		if out := tx.Tick(1180*ms, nil); len(out) != 1 || out[0].Hint.DataType != enums.DataTypeVoiceLCHeader {
			t.Fatalf("%s: second ChanAuth PDU %+v", name, out)
		}
	}
}

func TestTransmitter_ChanAuthSkipped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  voice.TransmitterConfig
		opts *voice.CallOptions
	}{
		{name: "impolite", cfg: voice.TransmitterConfig{Slot: enums.Slot1, ChanAuth: constants.TMSChanAuth, Access: enums.AccessImpolite}, opts: &voice.CallOptions{HangTime: true}},
		{name: "not in hang time", cfg: voice.TransmitterConfig{Slot: enums.Slot1, ChanAuth: constants.TMSChanAuth}, opts: &voice.CallOptions{}},
		{name: "no options", cfg: voice.TransmitterConfig{Slot: enums.Slot1, ChanAuth: constants.TMSChanAuth}},
		{name: "off", cfg: voice.TransmitterConfig{Slot: enums.Slot1}, opts: &voice.CallOptions{HangTime: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.cfg.Silence = vocoder.AMBESilenceSlot()
			tx := newTransmitter(t, &tc.cfg)
			if err := tx.Start(0, voice.Call{Target: 2, Source: 1}, tc.opts); err != nil || tx.State() != voice.TransmitterHeader {
				t.Fatalf("%v, state %v", err, tx.State())
			}
			own, _, _ := outbound(t, 1)
			if out := tx.Burst(0, &own, layer2.Hint{}, nil); len(out) != 1 || out[0].Hint.DataType != enums.DataTypeVoiceLCHeader {
				t.Fatalf("outputs %+v", out)
			}
		})
	}
	if _, err := voice.NewTransmitter(&voice.TransmitterConfig{Slot: enums.Slot1, ChanAuth: -1}); err == nil {
		t.Fatal("NewTransmitter took a negative ChanAuth")
	}
	if voice.TransmitterChanAuth.String() != "ChanAuth" {
		t.Error(voice.TransmitterChanAuth.String())
	}
}
