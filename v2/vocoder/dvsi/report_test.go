package dvsi_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/dvsisim"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi"
)

func TestCodecSpeechFormat(t *testing.T) {
	t.Parallel()

	d := dvsisim.New(dvsisim.Pairs(1), dvsisim.Options{})
	if _, err := dvsi.New(d, nil); err != nil {
		t.Fatal(err)
	}
	if d.Counts.SpeechFormat != 1 {
		t.Errorf("New sent %d PKT_SPCHFMT fields, want 1", d.Counts.SpeechFormat)
	}
	checkSim(t, d)

	d = dvsisim.New(dvsisim.Pairs(1), dvsisim.Options{RejectSpeechFormat: true})
	if _, err := dvsi.New(d, nil); !errors.Is(err, dvsi.ErrSpeechFormat) {
		t.Errorf("New with a rejected PKT_SPCHFMT field = %v", err)
	}
	checkSim(t, d)
}

func TestCodecReport(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(5)
	status := map[[9]byte]uint16{
		pairs[0].Slot: dvsisim.VoiceActive,
		pairs[1].Slot: 0,
		pairs[2].Slot: dvsisim.VoiceActive | dvsisim.ToneFrame,
		pairs[3].Slot: dvsisim.VoiceActive | dvsisim.DataInvalid,
		pairs[4].Slot: dvsisim.DataInvalid,
	}
	want := []vocoder.FrameReport{
		{Frame: vocoder.FrameGood, Output: vocoder.OutputVoice},
		{Frame: vocoder.FrameGood, Output: vocoder.OutputSilence},
		{Frame: vocoder.FrameGood, Output: vocoder.OutputTone},
		{Frame: vocoder.FrameErased, Output: vocoder.OutputRepeat},
		{Frame: vocoder.FrameErased, Output: vocoder.OutputSilence},
	}
	tests := []struct {
		name string
		opt  dvsisim.Options
		cfg  dvsi.Config
	}{
		{"CMODE after SPEECHD", dvsisim.Options{}, dvsi.Config{}},
		{"CMODE before SPEECHD", dvsisim.Options{CModeFirst: true}, dvsi.Config{}},
		{"no parity, one byte per read", dvsisim.Options{Chunk: 1, CModeFirst: true}, dvsi.Config{NoParity: true}},
		{"AMBE-3003 channel 2 datagrams", dvsisim.Options{Channels: 3, Datagram: true}, dvsi.Config{Channel: 2}},
		{"AMBE-3003 channel 1, CMODE before SPEECHD", dvsisim.Options{Channels: 3, CModeFirst: true}, dvsi.Config{Channel: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := dvsisim.New(pairs, tt.opt)
			d.Status = status
			c, err := dvsi.New(d, &tt.cfg)
			if err != nil {
				t.Fatal(err)
			}
			for i, p := range pairs {
				pcm, rep, err := c.Decode(nil, p.Slot)
				if err != nil || !slices.Equal(pcm, p.PCM[:]) {
					t.Fatalf("Decode %d = %v, right audio %v", i, err, slices.Equal(pcm, p.PCM[:]))
				}
				if rep != want[i] {
					t.Errorf("Decode %d: report %+v, want %+v", i, rep, want[i])
				}
			}
			checkSim(t, d)
		})
	}
}

func TestCodecDecodeLost(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(3)
	for _, opt := range []dvsisim.Options{{}, {CModeFirst: true, Chunk: 2}, {Datagram: true}, {Channels: 3}} {
		cfg := dvsi.Config{}
		if opt.Channels == 3 {
			cfg.Channel = 2
		}
		d := dvsisim.New(pairs, opt)
		c, err := dvsi.New(d, &cfg)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := c.Decode(nil, pairs[0].Slot); err != nil {
			t.Fatal(err)
		}
		want := []vocoder.FrameOutput{vocoder.OutputRepeat, vocoder.OutputRepeat, vocoder.OutputSilence}
		for i, w := range want {
			dst := make([]int16, 2, 2+dvsi.FrameSamples)
			pcm, rep, err := c.DecodeLost(dst)
			if err != nil || len(pcm) != 2+dvsi.FrameSamples || &pcm[0] != &dst[0] {
				t.Fatalf("%+v: DecodeLost %d = %d samples, %v", opt, i, len(pcm), err)
			}
			if !slices.Equal(pcm[2:], pairs[0].PCM[:]) {
				t.Errorf("%+v: DecodeLost %d did not repeat the frame before", opt, i)
			}
			if rep != (vocoder.FrameReport{Frame: vocoder.FrameLost, Output: w}) {
				t.Errorf("%+v: DecodeLost %d: report %+v", opt, i, rep)
			}
		}
		for i, p := range pairs[1:] {
			pcm, rep, err := c.Decode(nil, p.Slot)
			if err != nil || !slices.Equal(pcm, p.PCM[:]) {
				t.Fatalf("%+v: Decode %d after DecodeLost = %v, right audio %v", opt, i, err, slices.Equal(pcm, p.PCM[:]))
			}
			if rep != (vocoder.FrameReport{}) {
				t.Errorf("%+v: Decode %d after DecodeLost: report %+v", opt, i, rep)
			}
		}
		if d.Counts.Lost != 3 || d.Counts.CMode != 4 || d.Counts.Decode != 6 {
			t.Errorf("%+v: requests %+v, want 3 lost frames and 4 CMODE fields", opt, d.Counts)
		}
		checkSim(t, d)
	}
}

func TestCodecNoCMode(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(2)
	d := dvsisim.New(pairs, dvsisim.Options{NoCMode: true})
	d.Status = map[[9]byte]uint16{pairs[1].Slot: dvsisim.DataInvalid}
	c, err := dvsi.New(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pairs {
		pcm, rep, err := c.Decode(nil, p.Slot)
		if err != nil || !slices.Equal(pcm, p.PCM[:]) || rep != (vocoder.FrameReport{}) {
			t.Errorf("Decode without CMODE = %+v, %v", rep, err)
		}
	}
	pcm, rep, err := c.DecodeLost(nil)
	if err != nil || len(pcm) != dvsi.FrameSamples || rep != (vocoder.FrameReport{Frame: vocoder.FrameLost, Output: vocoder.OutputRepeat}) {
		t.Errorf("DecodeLost without CMODE = %d samples, %+v, %v", len(pcm), rep, err)
	}
	checkSim(t, d)
}

func TestCodecReportErrors(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(1)
	d := dvsisim.New(pairs, dvsisim.Options{})
	c, err := dvsi.New(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
	dst := []int16{1, 2}
	if out, rep, err := c.DecodeLost(dst); !errors.Is(err, dvsi.ErrClosed) || len(out) != 2 || rep != (vocoder.FrameReport{}) {
		t.Errorf("DecodeLost after Close = %d samples, %+v, %v", len(out), rep, err)
	}
	if out, _, err := c.Decode(dst, pairs[0].Slot); !errors.Is(err, dvsi.ErrClosed) || len(out) != 2 {
		t.Errorf("Decode after Close = %d samples, %v", len(out), err)
	}
	var z dvsi.Codec
	if _, _, err := z.DecodeLost(nil); !errors.Is(err, dvsi.ErrClosed) {
		t.Errorf("DecodeLost of the zero Codec = %v", err)
	}
	var n *dvsi.Codec
	if _, _, err := n.DecodeLost(nil); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("DecodeLost of a nil Codec = %v", err)
	}
	if _, _, err := n.Decode(nil, pairs[0].Slot); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("Decode of a nil Codec = %v", err)
	}
	checkSim(t, d)
}

type recordPort struct {
	*dvsisim.Device
	writes [][]byte
}

func (p *recordPort) Write(b []byte) (int, error) {
	p.writes = append(p.writes, append([]byte(nil), b...))
	return p.Device.Write(b)
}

func TestCodecStatusPackets(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(2)
	for _, ch := range []int{0, 1} {
		p := &recordPort{Device: dvsisim.New(pairs, dvsisim.Options{Channels: 3})}
		c, err := dvsi.New(p, &dvsi.Config{Channel: ch, NoParity: true})
		if err != nil {
			t.Fatal(err)
		}
		if got := p.writes[len(p.writes)-2]; !slices.Equal(got, []byte{0x61, 0x00, 0x03, 0x00, 0x16, 0x00, 0x01}) {
			t.Errorf("channel %d: PKT_SPCHFMT packet % x", ch, got)
		}
		var chField []byte
		if ch > 0 {
			chField = []byte{0x41}
		}
		n := byte(len(chField))
		want := slices.Concat([]byte{0x61, 0x00, 3 + n, 0x00}, chField, []byte{0x06, 0x00, 0x00})
		if got := p.writes[len(p.writes)-1]; !slices.Equal(got, want) {
			t.Errorf("channel %d: PKT_DCMODE packet % x, want % x", ch, got, want)
		}
		if _, _, err := c.DecodeLost(nil); err != nil {
			t.Fatal(err)
		}
		want = slices.Concat([]byte{0x61, 0x00, 3 + n, 0x01}, chField, []byte{0x02, 0x00, 0x04})
		if got := p.writes[len(p.writes)-1]; !slices.Equal(got, want) {
			t.Errorf("channel %d: lost frame packet % x, want % x", ch, got, want)
		}
		for i, s := range []byte{0x02, 0x00} {
			if _, _, err := c.Decode(nil, pairs[i].Slot); err != nil {
				t.Fatal(err)
			}
			want = slices.Concat([]byte{0x61, 0x00, 11 + n + 3*s/2, 0x01}, chField, []byte{0x02, 0x00, 0x00}[:3*s/2], []byte{0x01, 0x48}, pairs[i].Slot[:])
			if got := p.writes[len(p.writes)-1]; !slices.Equal(got, want) {
				t.Errorf("channel %d: decode %d packet % x, want % x", ch, i, got, want)
			}
		}
		checkSim(t, p.Device)
	}
}

func TestCodecLeftLostFrame(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(3)
	tests := []struct {
		name  string
		opt   dvsisim.Options
		cfg   dvsi.Config
		crash bool
	}{
		{"AMBE-3000R left by a client that crashed", dvsisim.Options{}, dvsi.Config{NoReset: true}, true},
		{"AMBEserver datagrams", dvsisim.Options{Datagram: true}, dvsi.Config{NoReset: true}, false},
		{"AMBE-3003 channel 1", dvsisim.Options{Channels: 3}, dvsi.Config{Channel: 1, NoReset: true}, false},
		{"AMBE-3003 channel 2 left by a client that crashed", dvsisim.Options{Channels: 3, Chunk: 3}, dvsi.Config{Channel: 2, NoReset: true, NoParity: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := dvsisim.New(pairs, tt.opt)
			old, err := dvsi.New(d, &dvsi.Config{Channel: tt.cfg.Channel, NoParity: tt.cfg.NoParity})
			if err != nil {
				t.Fatal(err)
			}
			if tt.crash {
				if _, _, err := old.Decode(nil, pairs[0].Slot); err != nil {
					t.Fatal(err)
				}
				if _, _, err := old.DecodeLost(nil); err != nil {
					t.Fatal(err)
				}
			} else {
				d.SetDCMode(tt.cfg.Channel, dvsisim.LostFrame)
			}
			if d.DCMode(tt.cfg.Channel) != dvsisim.LostFrame {
				t.Fatalf("DCMODE_IN %#x, want LOST_FRAME left set", d.DCMode(tt.cfg.Channel))
			}
			c, err := dvsi.New(d, &tt.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if d.DCMode(tt.cfg.Channel) != 0 {
				t.Errorf("DCMODE_IN %#x after New, want 0", d.DCMode(tt.cfg.Channel))
			}
			lost := d.Counts.Lost
			for i, p := range pairs {
				pcm, rep, err := c.Decode(nil, p.Slot)
				if err != nil {
					t.Fatalf("Decode %d: %v", i, err)
				}
				if !slices.Equal(pcm, p.PCM[:]) || rep != (vocoder.FrameReport{}) {
					t.Errorf("Decode %d: right audio %v, report %+v", i, slices.Equal(pcm, p.PCM[:]), rep)
				}
			}
			if d.Counts.Lost != lost {
				t.Errorf("the device repeated %d frames", d.Counts.Lost-lost)
			}
			checkSim(t, d)
		})
	}
}

func TestCodecDCModeRejected(t *testing.T) {
	t.Parallel()

	for _, opt := range []dvsisim.Options{{RejectDCMode: true}, {RejectDCMode: true, Channels: 3, Datagram: true}} {
		d := dvsisim.New(dvsisim.Pairs(1), opt)
		cfg := dvsi.Config{}
		if opt.Channels == 3 {
			cfg.Channel = 2
		}
		c, err := dvsi.New(d, &cfg)
		if c != nil || !errors.Is(err, dvsi.ErrStatus) || errors.Is(err, dvsi.ErrSpeechFormat) || !strings.Contains(err.Error(), "PKT_DCMODE") {
			t.Errorf("%+v: New with a rejected PKT_DCMODE field = codec %v, %v", opt, c != nil, err)
		}
		if d.Counts.DCMode != 1 {
			t.Errorf("%+v: New sent %d PKT_DCMODE fields, want 1", opt, d.Counts.DCMode)
		}
		checkSim(t, d)
	}
}

func TestCodecCloseClearsLostFrame(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(2)
	for _, opt := range []dvsisim.Options{{}, {Datagram: true}, {Channels: 3, Chunk: 1}} {
		cfg := dvsi.Config{}
		if opt.Channels == 3 {
			cfg.Channel = 1
		}
		d := dvsisim.New(pairs, opt)
		c, err := dvsi.New(d, &cfg)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := c.Decode(nil, pairs[0].Slot); err != nil {
			t.Fatal(err)
		}
		if _, _, err := c.DecodeLost(nil); err != nil {
			t.Fatal(err)
		}
		sent := d.Counts.DCMode
		if err := c.Close(); err != nil {
			t.Errorf("%+v: Close after DecodeLost = %v", opt, err)
		}
		if d.DCMode(cfg.Channel) != 0 || d.Counts.DCMode != sent+1 {
			t.Errorf("%+v: DCMODE_IN %#x after Close, %d PKT_DCMODE fields sent by Close", opt, d.DCMode(cfg.Channel), d.Counts.DCMode-sent)
		}
		if err := c.Close(); err != nil || d.Counts.DCMode != sent+1 {
			t.Errorf("%+v: second Close = %v, %d PKT_DCMODE fields", opt, err, d.Counts.DCMode-sent)
		}
		checkSim(t, d)

		d = dvsisim.New(pairs, opt)
		c, err = dvsi.New(d, &cfg)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := c.DecodeLost(nil); err != nil {
			t.Fatal(err)
		}
		if _, _, err := c.Decode(nil, pairs[1].Slot); err != nil {
			t.Fatal(err)
		}
		sent = d.Counts.DCMode
		if err := c.Close(); err != nil || d.Counts.DCMode != sent || d.DCMode(cfg.Channel) != 0 {
			t.Errorf("%+v: Close after Decode = %v, %d PKT_DCMODE fields, DCMODE_IN %#x", opt, err, d.Counts.DCMode-sent, d.DCMode(cfg.Channel))
		}
		checkSim(t, d)
	}
}

func TestCodecCloseErrors(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(1)
	d := dvsisim.New(pairs, dvsisim.Options{})
	c, err := dvsi.New(d, &dvsi.Config{Timeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.DecodeLost(nil); err != nil {
		t.Fatal(err)
	}
	d.Drop = 1
	if err := c.Close(); !errors.Is(err, dvsi.ErrTimeout) {
		t.Errorf("Close with no reply = %v", err)
	}
	if _, _, err := c.Decode(nil, pairs[0].Slot); !errors.Is(err, dvsi.ErrClosed) {
		t.Errorf("Decode after a failed Close = %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close = %v", err)
	}
	checkSim(t, d)

	boom := errors.New("boom")
	fp := &failPort{Device: dvsisim.New(pairs, dvsisim.Options{})}
	c, err = dvsi.New(fp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.DecodeLost(nil); err != nil {
		t.Fatal(err)
	}
	fp.err = boom
	if err := c.Close(); !errors.Is(err, boom) {
		t.Errorf("Close with a failing Write = %v", err)
	}
	checkSim(t, fp.Device)
}

type failPort struct {
	*dvsisim.Device
	err error
}

func (p *failPort) Write(b []byte) (int, error) {
	if p.err != nil {
		return 0, p.err
	}
	return p.Device.Write(b)
}
