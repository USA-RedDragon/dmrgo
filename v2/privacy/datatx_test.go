package privacy_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/privacy"
)

const captureMI = 0x0C01A418

func captureMessage(t *testing.T, c *dataCapture) (*privacy.Transmitter, *data.Message) {
	t.Helper()
	ptx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: privacy.NewARC4Key(1, c.key), Service: privacy.ServiceLandMobile})
	if err != nil {
		t.Fatal(err)
	}
	m := &data.Message{Rate: enums.DataTypeRate12, Payload: bytes.Clone(c.plain)}
	if err := m.Header.SetUnconfirmed(pdu.UnconfirmedDataHeader{
		Group: true, SAP: enums.SAPProprietaryPacketData, LLIDDestination: 1, LLIDSource: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	return ptx, m
}

func sendCapture(t *testing.T, m *data.Message) []layer2.RawBurst {
	t.Helper()
	s, err := data.NewSender(&data.SenderConfig{Slot: enums.Slot1, ColorCode: 1, Source: layer2.SyncSourceMS, Preambles: -1})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(0, m); err != nil {
		t.Fatal(err)
	}
	var bursts []layer2.RawBurst
	out := make([]data.Output, 0, data.MaxOutputs)
	for {
		at, ok := s.Deadline()
		if !ok {
			return bursts
		}
		out = s.Tick(at, out[:0])
		for _, o := range out {
			if o.Kind == data.OutputBurst {
				bursts = append(bursts, o.Burst)
			}
		}
	}
}

func TestTransmitterMatchesDataCapture(t *testing.T) {
	t.Parallel()

	c := loadDataCapture(t)
	ptx, m := captureMessage(t, c)
	ph, err := ptx.Data(captureMI, enums.SAPUDPIPHeaderCompression, m.Payload)
	if err != nil {
		t.Fatal(err)
	}
	m.Proprietary = &ph
	want := make([][12]byte, 0, 2+len(c.blocks))
	want = append(want, c.header, c.phead)
	want = append(want, c.blocks...)
	got := sendCapture(t, m)
	if len(got) != len(want) {
		t.Fatalf("%d bursts, the capture has %d", len(got), len(want))
	}
	for i, p := range want {
		dt := enums.DataTypeRate12
		if i < 2 {
			dt = enums.DataTypeDataHeader
		}
		raw, err := layer2.BuildDataBurst(p, dt, 1, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		if got[i] != raw {
			t.Errorf("burst %d: %X, the capture has %X (payload %X)", i, got[i], raw, p)
		}
	}

	rx, err := data.NewReceiver(nil)
	if err != nil {
		t.Fatal(err)
	}
	prx := privacy.NewReceiver()
	if err := prx.AddKey(privacy.NewARC4Key(1, c.key)); err != nil {
		t.Fatal(err)
	}
	events := make([]data.Event, 0, data.MaxEvents)
	delivered := false
	for i := range got {
		events = rx.Burst(time.Duration(i)*60*time.Millisecond, &got[i], layer2.Hint{}, events[:0])
		for j := range events {
			if events[j].Kind != data.EventDelivered {
				continue
			}
			delivered = true
			dm := events[j].Message
			in := prx.Data(&dm)
			if !in.Decrypting || in.MI != captureMI || in.KeyID != 1 || in.Algorithm != privacy.AlgorithmARC4 || dm.BadCRC {
				t.Errorf("info %+v", in)
			}
			if !bytes.Equal(dm.Payload, c.plain) {
				t.Errorf("payload %X, want %X", dm.Payload, c.plain)
			}
			if dm.Proprietary.SAP != enums.SAPUDPIPHeaderCompression {
				t.Errorf("P_HEAD SAP %v", dm.Proprietary.SAP)
			}
		}
	}
	if !delivered {
		t.Error("the own receiver delivered nothing")
	}
}

func TestTransmitterDataRoundTrip(t *testing.T) {
	t.Parallel()

	key := privacy.NewARC4Key(9, [5]byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5})
	ptx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: key, Service: privacy.ServiceLicenseFree})
	if err != nil {
		t.Fatal(err)
	}
	prx := privacy.NewReceiver()
	if err := prx.AddKey(key); err != nil {
		t.Fatal(err)
	}
	for n := range 300 {
		plain := make([]byte, n)
		for i := range plain {
			plain[i] = byte(i * 7)
		}
		p := bytes.Clone(plain)
		mi := uint32(n)*0x9E3779B9 + 1
		ph, err := ptx.Data(mi, enums.SAPShortData, p)
		if err != nil {
			t.Fatal(err)
		}
		if n >= 4 && bytes.Equal(p, plain) {
			t.Errorf("%d octets: payload in the clear", n)
		}
		want := privacy.Info{Encrypted: true, Header: true, MFID: enums.FIDMotorola, Algorithm: privacy.AlgorithmARC4, KeyID: 9, MI: mi}
		if got := privacy.FromProprietaryHeader(&ph); got != want || ph.SAP != enums.SAPShortData {
			t.Fatalf("%d octets: %+v, SAP %v", n, got, ph.SAP)
		}
		m := data.Message{Proprietary: &ph, Payload: p}
		if in := prx.Data(&m); !in.Decrypting || !bytes.Equal(m.Payload, plain) {
			t.Fatalf("%d octets: %+v %X", n, in, m.Payload)
		}
	}
}

func TestTransmitterDataErrors(t *testing.T) {
	t.Parallel()

	p := []byte{1, 2, 3}
	var nt *privacy.Transmitter
	if _, err := nt.Data(1, enums.SAPShortData, p); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil Transmitter: %v", err)
	}
	aes, err := privacy.NewAESKey(1, make([]byte, 16))
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []*privacy.Key{aes, privacy.NewBasicKey(0x1234)} {
		ptx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: k, Service: privacy.ServiceLandMobile})
		if err != nil {
			t.Fatal(err)
		}
		if h, err := ptx.Data(1, enums.SAPShortData, p); !errors.Is(err, privacy.ErrDataAlgorithm) || h != (pdu.ProprietaryHeader{}) {
			t.Errorf("%v: %v %+v", k.Algorithm(), err, h)
		}
	}
	ptx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: privacy.NewARC4Key(1, [5]byte{1}), Service: privacy.ServiceLandMobile})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ptx.Data(1, 16, p); !errors.As(err, new(*dmrgo.ConfigError)) {
		t.Errorf("SAP 16: %v", err)
	}
	if !bytes.Equal(p, []byte{1, 2, 3}) {
		t.Errorf("a failed Data changed the payload: %X", p)
	}
	if _, err := ptx.Data(1, enums.SAPShortData, nil); err != nil {
		t.Errorf("no payload: %v", err)
	}
}

func TestTransmitterDataAmateur(t *testing.T) {
	t.Parallel()

	for _, k := range []*privacy.Key{privacy.NewARC4Key(1, [5]byte{1}), privacy.NewBasicKey(1)} {
		ptx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: k, Service: privacy.ServiceAmateur})
		if !errors.Is(err, privacy.ErrAmateur) || ptx != nil {
			t.Fatalf("%v in the amateur service: %v", k.Algorithm(), err)
		}
		p := []byte{1, 2, 3}
		if _, err := ptx.Data(1, enums.SAPShortData, p); err == nil || !bytes.Equal(p, []byte{1, 2, 3}) {
			t.Errorf("%v: the Transmitter of the amateur service encrypted data: %v", k.Algorithm(), err)
		}
	}
}
