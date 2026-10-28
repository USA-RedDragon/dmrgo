package privacy_test

import (
	"bufio"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/privacy"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

type capture struct {
	name   string
	key    *privacy.Key
	pi     pdu.PIHeader
	hasPI  bool
	ct, pt []vocoder.AMBEFrame
}

func frame(t *testing.T, s string) vocoder.AMBEFrame {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 7 {
		t.Fatalf("frame %q: %v", s, err)
	}
	return vocoder.AMBEFrame(b)
}

func loadCapture(t *testing.T, name string, key *privacy.Key) *capture {
	t.Helper()
	f, err := os.Open("testdata/" + name + ".txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	c := &capture{name: name, key: key}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		switch {
		case len(fields) == 2 && fields[0] == "pi":
			if fields[1] == "none" {
				continue
			}
			b, err := hex.DecodeString(fields[1])
			if err != nil || len(b) != 12 {
				t.Fatalf("pi %q: %v", fields[1], err)
			}
			c.pi, c.hasPI = pdu.DecodePIHeader([12]byte(b)), true
			if c.pi.FEC.Uncorrectable {
				t.Fatalf("%s: PI header CRC fails", name)
			}
		case len(fields) == 2 && fields[0] == "lc":
		case len(fields) == 2:
			c.ct = append(c.ct, frame(t, fields[0]))
			c.pt = append(c.pt, frame(t, fields[1]))
		default:
			t.Fatalf("%s: bad line %q", name, sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return c
}

func aesKey(t *testing.T, id uint8, s string) *privacy.Key {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	k, err := privacy.NewAESKey(id, b)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func captures(t *testing.T) []*capture {
	t.Helper()
	return []*capture{
		loadCapture(t, "rc4", privacy.NewARC4Key(0x0F, [5]byte{0x37, 0x33, 0x74, 0xAB, 0xE8})),
		loadCapture(t, "aes128", aesKey(t, 0x0D, "373374ABE8E9D13AA90900B9A3101664")),
		loadCapture(t, "aes256", aesKey(t, 0x0E, "3AF8C96E21BEFDE927EE1D03DF53CFA701516B0642AC7B38643549E2BE79840E")),
		loadCapture(t, "bp", privacy.NewBasicKeyNumber(100)),
	}
}

var encryptedCall = voice.Call{Group: true, Target: 1, Source: 1, Options: layer3.ServiceOptions{IsPrivacy: true}, FID: enums.FIDMotorola}

func newVoiceTransmitter(t *testing.T) *voice.Transmitter {
	t.Helper()
	tx, err := voice.NewTransmitter(&voice.TransmitterConfig{Slot: enums.Slot1,
		ColorCode: 1, Source: layer2.SyncSourceMS, Silence: vocoder.AMBESilenceSlot(),
		Access: enums.AccessImpolite, Queue: voice.MaxQueue,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

func transmit(t *testing.T, call voice.Call, frames []vocoder.AMBEFrame, pi *pdu.PIHeader, each func(o *voice.Output)) {
	t.Helper()
	tx := newVoiceTransmitter(t)
	if err := tx.Start(0, call, &voice.CallOptions{PIHeader: pi}); err != nil {
		t.Fatal(err)
	}
	out := make([]voice.Output, 0, voice.MaxOutputs)
	next, stopped := 0, false
	for now := time.Duration(0); now < 10*time.Minute; now += 60 * time.Millisecond {
		for next < len(frames) && tx.Room() > 0 {
			if err := tx.Frame(frames[next]); err != nil {
				t.Fatal(err)
			}
			next++
		}
		if next == len(frames) && !stopped {
			tx.Stop(now)
			stopped = true
		}
		outs := tx.Tick(now, out[:0])
		for i := range outs {
			o := &outs[i]
			if o.Kind == voice.OutputEnded {
				return
			}
			each(o)
		}
	}
	t.Fatal("the call did not end")
}

func receive(t *testing.T, ct []vocoder.AMBEFrame, pi *pdu.PIHeader, prx *privacy.Receiver) (got []vocoder.AMBEFrame, info []privacy.Info) {
	t.Helper()
	got, info, _ = receiveWith(t, encryptedCall, ct, pi, prx, true, nil)
	return got, info
}

func receiveWith(t *testing.T, call voice.Call, ct []vocoder.AMBEFrame, pi *pdu.PIHeader, prx *privacy.Receiver, hint bool, drop func(int) bool) (got []vocoder.AMBEFrame, info []privacy.Info, sent []int) {
	t.Helper()
	rx, err := voice.NewReceiver(nil)
	if err != nil {
		t.Fatal(err)
	}
	events := make([]voice.Event, 0, voice.MaxEvents)
	n := 0
	transmit(t, call, ct, pi, func(o *voice.Output) {
		h := o.Hint
		if o.Hint.Kind == layer2.KindVoice {
			n++
			if drop != nil && drop(n-1) {
				return
			}
			sent = append(sent, n-1)
		}
		if !hint {
			h = layer2.Hint{}
		}
		events = rx.Burst(o.Due, &o.Burst, h, events[:0])
		for i := range events {
			var f [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
			in := prx.Event(&events[i], &f)
			info = append(info, in)
			if events[i].Kind == voice.EventVoice {
				got = append(got, f[:]...)
			}
		}
	})
	return got, info, sent
}

func TestReceiverLostBursts(t *testing.T) {
	t.Parallel()

	for _, hint := range []bool{true, false} {
		for _, c := range captures(t)[:3] {
			prx := privacy.NewReceiver()
			if err := prx.AddKey(c.key); err != nil {
				t.Fatal(err)
			}
			got, _, sent := receiveWith(t, encryptedCall, c.ct, &c.pi, prx, hint, func(i int) bool { return i >= 4 && i < 12 || i == 20 })
			if len(got) != 3*len(sent) {
				t.Fatalf("%s hint %v: %d frames for %d bursts", c.name, hint, len(got), len(sent))
			}
			for j, b := range sent {
				for k := range 3 {
					if n := 3*b + k; n < len(c.pt) && got[3*j+k] != c.pt[n] {
						t.Fatalf("%s hint %v: burst %d frame %d: %X, want %X", c.name, hint, b, k, got[3*j+k], c.pt[n])
					}
				}
			}
		}
	}
}

func TestReceiverDecryptsCaptures(t *testing.T) {
	t.Parallel()

	for _, c := range captures(t) {
		prx := privacy.NewReceiver()
		if err := prx.AddKey(c.key); err != nil {
			t.Fatal(err)
		}
		var pi *pdu.PIHeader
		if c.hasPI {
			pi = &c.pi
		}
		got, infos := receive(t, c.ct, pi, prx)
		if len(got) < len(c.pt) {
			t.Fatalf("%s: %d frames, want %d", c.name, len(got), len(c.pt))
		}
		bad := 0
		for i := range c.pt {
			if got[i] != c.pt[i] {
				if bad < 3 {
					t.Errorf("%s frame %d: %X, want %X", c.name, i, got[i], c.pt[i])
				}
				bad++
			}
		}
		if bad > 0 {
			t.Errorf("%s: %d of %d frames differ", c.name, bad, len(c.pt))
		}
		last := infos[len(infos)-1]
		if !last.Encrypted || !last.Decrypting || last.Header != c.hasPI {
			t.Errorf("%s: info %+v", c.name, last)
		}
		if c.hasPI && (last.Algorithm != c.key.Algorithm() || last.KeyID != c.key.ID() || last.MI != c.pi.MessageIndicator || last.MFID != enums.FIDMotorola) {
			t.Errorf("%s: info %+v, want algorithm %v key ID %d MI %08X", c.name, last, c.key.Algorithm(), c.key.ID(), c.pi.MessageIndicator)
		}
	}
}

func TestReceiverReportsWithoutKey(t *testing.T) {
	t.Parallel()

	for _, c := range captures(t)[:3] {
		got, infos := receive(t, c.ct, &c.pi, privacy.NewReceiver())
		for i := range c.ct {
			if got[i] != c.ct[i] {
				t.Fatalf("%s frame %d: %X, want it as received, %X", c.name, i, got[i], c.ct[i])
			}
		}
		want := privacy.Info{Encrypted: true, Header: true, MFID: enums.FIDMotorola, Algorithm: c.key.Algorithm(), KeyID: c.key.ID(), MI: c.pi.MessageIndicator}
		if last := infos[len(infos)-1]; last != want {
			t.Errorf("%s: info %+v, want %+v", c.name, last, want)
		}
	}
}

func TestReceiverWrongKeyID(t *testing.T) {
	t.Parallel()

	c := captures(t)[0]
	prx := privacy.NewReceiver()
	if err := prx.AddKey(privacy.NewARC4Key(c.key.ID()+1, [5]byte{0x37, 0x33, 0x74, 0xAB, 0xE8})); err != nil {
		t.Fatal(err)
	}
	got, infos := receive(t, c.ct, &c.pi, prx)
	if got[0] != c.ct[0] || infos[len(infos)-1].Decrypting {
		t.Errorf("a key with another key ID decrypts: %X, %+v", got[0], infos[len(infos)-1])
	}
}

func TestTransmitterEncryptsCaptures(t *testing.T) {
	t.Parallel()

	for _, c := range captures(t) {
		ptx, err := privacy.NewTransmitter(&privacy.TransmitterConfig{Key: c.key, Service: privacy.ServiceLandMobile})
		if err != nil {
			t.Fatal(err)
		}
		ptx.Start(c.pi.MessageIndicator)
		pi, ok := ptx.PIHeader(1)
		if ok != c.hasPI {
			t.Fatalf("%s: PIHeader ok = %v", c.name, ok)
		}
		var hdr *pdu.PIHeader
		if ok {
			want := c.pi
			want.FEC, want.Raw = pi.FEC, pi.Raw
			if pi != want {
				t.Errorf("%s: PI header %+v, want %+v", c.name, pi, want)
			}
			hdr = &pi
		}
		var got []vocoder.AMBEFrame
		sawPI := false
		transmit(t, encryptedCall, c.pt, hdr, func(o *voice.Output) {
			if err := ptx.Output(o); err != nil {
				t.Fatal(err)
			}
			if o.Hint.Kind == layer2.KindData && o.Hint.DataType == enums.DataTypePIHeader {
				sawPI = true
			}
			if o.Hint.Kind == layer2.KindVoice {
				f, _ := o.Burst.AMBEFrames()
				got = append(got, f[:]...)
			}
		})
		if sawPI != c.hasPI {
			t.Errorf("%s: PI header sent = %v", c.name, sawPI)
		}
		bad := 0
		for i := range c.ct {
			if got[i] != c.ct[i] {
				if bad < 3 {
					t.Errorf("%s frame %d: %X, want %X", c.name, i, got[i], c.ct[i])
				}
				bad++
			}
		}
		if bad > 0 {
			t.Errorf("%s: %d of %d frames differ", c.name, bad, len(c.ct))
		}
	}
}

func TestReceiverBasicLateLC(t *testing.T) {
	t.Parallel()

	c := captures(t)[3]
	prx := privacy.NewReceiver()
	if err := prx.AddKey(c.key); err != nil {
		t.Fatal(err)
	}
	var raw [2]layer2.RawBurst
	for i := range raw {
		var err error
		raw[i], err = layer2.BuildVoiceBurst([3]vocoder.AMBEFrame(c.ct[3*i:3*i+3]), enums.VoiceBurstA, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
	}
	var f [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	if in := prx.Event(&voice.Event{Kind: voice.EventVoice, Burst: raw[0]}, &f); in.Decrypting || f[0] != c.ct[0] {
		t.Errorf("before the LC: %+v %X", in, f[0])
	}
	prx.Event(&voice.Event{Kind: voice.EventCallStart, Call: encryptedCall}, nil)
	if in := prx.Event(&voice.Event{Kind: voice.EventVoice, Burst: raw[1]}, &f); !in.Decrypting || f[0] != c.pt[3] {
		t.Errorf("after the LC: %+v %X, want %X", in, f[0], c.pt[3])
	}
}

func TestReceiverForceBasic(t *testing.T) {
	t.Parallel()

	c := captures(t)[3]
	plain := voice.Call{Group: true, Target: 1, Source: 1}
	for _, force := range []bool{false, true} {
		prx := privacy.NewReceiver()
		if err := prx.AddKey(c.key); err != nil {
			t.Fatal(err)
		}
		prx.ForceBasic(force)
		got, infos, _ := receiveWith(t, plain, c.ct, nil, prx, true, nil)
		want := c.ct
		if force {
			want = c.pt
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("force %v frame %d: %X, want %X", force, i, got[i], want[i])
			}
		}
		last := infos[len(infos)-1]
		if last.Decrypting != force || last.Encrypted || last.Header {
			t.Errorf("force %v: info %+v", force, last)
		}
	}
}
