package dvsi_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/dvsisim"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi"
)

func checkSim(t *testing.T, d *dvsisim.Device) {
	t.Helper()
	if len(d.Errs) > 0 {
		t.Errorf("the device found errors: %q", d.Errs)
	}
}

func roundTrip(t *testing.T, c *dvsi.Codec, pairs []dvsisim.Pair) {
	t.Helper()
	for _, p := range pairs {
		pcm, _, err := c.Decode(nil, p.Slot)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if !slices.Equal(pcm, p.PCM[:]) {
			t.Fatalf("Decode of %x returned other samples", p.Slot)
		}
		slot, err := c.Encode(p.PCM[:])
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		if slot != p.Slot {
			t.Fatalf("Encode = %x, want %x", slot, p.Slot)
		}
	}
}

func TestCodecPorts(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(8)
	tests := []struct {
		name string
		opt  dvsisim.Options
		cfg  dvsi.Config
	}{
		{"serial", dvsisim.Options{}, dvsi.Config{}},
		{"serial, one byte per read", dvsisim.Options{Chunk: 1}, dvsi.Config{}},
		{"serial, 7 bytes per read", dvsisim.Options{Chunk: 7}, dvsi.Config{}},
		{"AMBEserver datagrams", dvsisim.Options{Datagram: true}, dvsi.Config{}},
		{"no parity", dvsisim.Options{}, dvsi.Config{NoParity: true}},
		{"no parity, one byte per read", dvsisim.Options{Chunk: 1}, dvsi.Config{NoParity: true}},
		{"parity pin low", dvsisim.Options{NoParityPin: true}, dvsi.Config{}},
		{"AMBE-3003 channel 2", dvsisim.Options{Channels: 3}, dvsi.Config{Channel: 2}},
		{"AMBE-3003 channel 1 datagrams", dvsisim.Options{Channels: 3, Datagram: true}, dvsi.Config{Channel: 1, NoParity: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := dvsisim.New(pairs, tt.opt)
			c, err := dvsi.New(d, &tt.cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if !d.Configured(tt.cfg.Channel) || d.Parity() == tt.cfg.NoParity {
				t.Errorf("channel configured %v, parity %v", d.Configured(tt.cfg.Channel), d.Parity())
			}
			wantProd := dvsisim.Product3000
			if tt.opt.Channels == 3 {
				wantProd = dvsisim.Product3003
			}
			if c.Product() != wantProd || c.Version() != dvsisim.Version {
				t.Errorf("product %q, version %q", c.Product(), c.Version())
			}
			roundTrip(t, c, pairs)
			checkSim(t, d)
			if d.Counts.Reset != 1 || d.Counts.Rate != 1 || d.Counts.Decode != len(pairs) || d.Counts.Encode != len(pairs) {
				t.Errorf("requests %v", d.Counts)
			}
		})
	}
}

func TestCodecInterface(t *testing.T) {
	t.Parallel()

	d := dvsisim.New(dvsisim.Pairs(1), dvsisim.Options{})
	c, err := dvsi.New(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	var vc vocoder.Codec = c
	if vc.FrameSamples() != 160 || vc.FID() != enums.FIDStandardized || vc.Silence() != vocoder.AMBESilenceSlot() {
		t.Errorf("FrameSamples %d, FID %d, Silence %x", vc.FrameSamples(), vc.FID(), vc.Silence())
	}
	dst := make([]int16, 3, 3+160)
	out, _, err := c.Decode(dst, dvsisim.Pairs(1)[0].Slot)
	if err != nil || len(out) != 163 || &out[0] != &dst[0] {
		t.Errorf("Decode = %d samples, %v", len(out), err)
	}
	if _, err := c.Encode(make([]int16, 159)); !errors.Is(err, dvsi.ErrSamples) {
		t.Errorf("Encode of 159 samples = %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Decode(nil, [9]byte{}); !errors.Is(err, dvsi.ErrClosed) {
		t.Errorf("Decode after Close = %v", err)
	}
	if _, err := c.Encode(make([]int16, 160)); !errors.Is(err, dvsi.ErrClosed) {
		t.Errorf("Encode after Close = %v", err)
	}
	checkSim(t, d)
}

func TestCodecZero(t *testing.T) {
	t.Parallel()

	var c dvsi.Codec
	if _, _, err := c.Decode(nil, [9]byte{}); !errors.Is(err, dvsi.ErrClosed) {
		t.Errorf("Decode of the zero Codec = %v", err)
	}
	if _, err := c.Encode(make([]int16, 160)); !errors.Is(err, dvsi.ErrClosed) {
		t.Errorf("Encode of the zero Codec = %v", err)
	}
	if c.Product() != "" || c.Version() != "" || c.Close() != nil {
		t.Error("the zero Codec has a product, a version or a Close error")
	}
}

func TestCodecNoReset(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(4)
	d := dvsisim.New(pairs, dvsisim.Options{Channels: 3})
	c0, err := dvsi.New(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	c1, err := dvsi.New(d, &dvsi.Config{Channel: 1, NoReset: true})
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, c0, pairs)
	roundTrip(t, c1, pairs)
	if d.Counts.Reset != 1 || !d.Configured(0) || !d.Configured(1) || d.Configured(2) {
		t.Errorf("requests %v", d.Counts)
	}
	checkSim(t, d)
}

func TestCodecResync(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(4)
	garbage := [][]byte{
		{0x00, 0xFF, 0x13},
		{0x61},
		{0x61, 0x00},
		{0x61, 0x00, 0x05, 0x02, 0x00},
		{0x61, 0x00, 0x02, 0x07, 0x00, 0x00},
		{0x61, 0x7F, 0xFF, 0x02},
		{0x61, 0x00, 0x03, 0x02, 0x00, 0x2F, 0x00},
		{0x61, 0x00, 0x02, 0x00, 0x3F, 0x00},
	}
	for _, opt := range []dvsisim.Options{{}, {Chunk: 1}, {Datagram: true}} {
		for _, parity := range []bool{true, false} {
			d := dvsisim.New(pairs, opt)
			c, err := dvsi.New(d, &dvsi.Config{NoParity: !parity, Timeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			for i, g := range garbage {
				for _, p := range pairs {
					d.Garbage = g
					pcm, _, err := c.Decode(nil, p.Slot)
					if err != nil || !slices.Equal(pcm, p.PCM[:]) {
						t.Fatalf("%+v parity %v, garbage %d %x: Decode = %v", opt, parity, i, g, err)
					}
					d.Garbage = g
					slot, err := c.Encode(p.PCM[:])
					if err != nil || slot != p.Slot {
						t.Fatalf("%+v parity %v, garbage %d %x: Encode = %x, %v", opt, parity, i, g, slot, err)
					}
				}
			}
			checkSim(t, d)
		}
	}
}

func TestCodecTimeout(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(2)
	d := dvsisim.New(pairs, dvsisim.Options{})
	c, err := dvsi.New(d, &dvsi.Config{Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	d.Drop = 1
	start := time.Now()
	if _, _, err := c.Decode(nil, pairs[0].Slot); !errors.Is(err, dvsi.ErrTimeout) {
		t.Fatalf("Decode with a lost reply = %v", err)
	}
	if el := time.Since(start); el < 20*time.Millisecond || el > time.Second {
		t.Errorf("timed out after %v", el)
	}
	d.Drop = 1
	if _, err := c.Encode(pairs[0].PCM[:]); !errors.Is(err, dvsi.ErrTimeout) {
		t.Fatalf("Encode with a lost reply = %v", err)
	}
	roundTrip(t, c, pairs)

	d = dvsisim.New(pairs, dvsisim.Options{})
	d.Drop = 1
	if _, err := dvsi.New(d, &dvsi.Config{Timeout: 10 * time.Millisecond}); !errors.Is(err, dvsi.ErrTimeout) {
		t.Errorf("New without PKT_READY = %v", err)
	}
	d = dvsisim.New(pairs, dvsisim.Options{})
	d.Drop = 2
	if _, err := dvsi.New(d, &dvsi.Config{Timeout: 10 * time.Millisecond}); !errors.Is(err, dvsi.ErrTimeout) {
		t.Errorf("New without a PARITYMODE response = %v", err)
	}
	checkSim(t, d)
}

func TestCodecLateReply(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(6)
	for _, opt := range []dvsisim.Options{{}, {Chunk: 3}, {Datagram: true}} {
		d := dvsisim.New(pairs, opt)
		c, err := dvsi.New(d, &dvsi.Config{Timeout: 20 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		d.Late = 1
		if _, _, err := c.Decode(nil, pairs[0].Slot); !errors.Is(err, dvsi.ErrTimeout) {
			t.Fatalf("%+v: Decode with a late reply = %v", opt, err)
		}
		for i, p := range pairs[1:] {
			pcm, _, err := c.Decode(nil, p.Slot)
			if err != nil || !slices.Equal(pcm, p.PCM[:]) {
				t.Fatalf("%+v: Decode %d after a late reply = %v, right audio %v", opt, i+1, err, slices.Equal(pcm, p.PCM[:]))
			}
		}
		d.Late = 1
		if _, err := c.Encode(pairs[0].PCM[:]); !errors.Is(err, dvsi.ErrTimeout) {
			t.Fatalf("%+v: Encode with a late reply = %v", opt, err)
		}
		for i, p := range pairs[1:] {
			slot, err := c.Encode(p.PCM[:])
			if err != nil || slot != p.Slot {
				t.Fatalf("%+v: Encode %d after a late reply = %x, %v", opt, i+1, slot, err)
			}
		}
		checkSim(t, d)
	}
}

type deadlinePort struct {
	*dvsisim.Device
	deadlines int
	fail      error
}

func (p *deadlinePort) SetReadDeadline(time.Time) error {
	p.deadlines++
	return p.fail
}

type brokenPort struct {
	readErr, writeErr error
	writeAll          bool
	readN             int
}

func (p *brokenPort) Read([]byte) (int, error) { return p.readN, p.readErr }

func (p *brokenPort) Write(b []byte) (int, error) {
	if p.writeErr != nil {
		return 0, p.writeErr
	}
	if p.writeAll {
		return len(b), nil
	}
	return 0, nil
}

func TestCodecPortErrors(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(1)
	dp := &deadlinePort{Device: dvsisim.New(pairs, dvsisim.Options{Chunk: 5})}
	c, err := dvsi.New(dp, nil)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip(t, c, pairs)
	if dp.deadlines == 0 {
		t.Error("SetReadDeadline was not called")
	}
	boom := errors.New("boom")
	nd := &deadlinePort{Device: dvsisim.New(pairs, dvsisim.Options{}), fail: boom}
	c2, err := dvsi.New(nd, nil)
	if err != nil {
		t.Fatalf("New on a port without deadlines: %v", err)
	}
	roundTrip(t, c2, pairs)
	if nd.deadlines != 1 {
		t.Errorf("SetReadDeadline called %d times on a port that refused the first", nd.deadlines)
	}
	dp.fail = boom
	if _, _, err := c.Decode(nil, pairs[0].Slot); !errors.Is(err, boom) {
		t.Errorf("Decode with a failing SetReadDeadline = %v", err)
	}

	if _, err := dvsi.New(&brokenPort{writeErr: boom}, nil); !errors.Is(err, boom) {
		t.Errorf("New with a failing Write = %v", err)
	}
	if _, err := dvsi.New(&brokenPort{}, nil); !errors.Is(err, dvsi.ErrPort) {
		t.Errorf("New with a Write of 0 bytes = %v", err)
	}
	if _, err := dvsi.New(&brokenPort{readErr: boom, writeAll: true}, nil); !errors.Is(err, boom) {
		t.Errorf("New with a failing Read = %v", err)
	}
	if _, err := dvsi.New(&brokenPort{readN: 1 << 20, writeAll: true}, nil); !errors.Is(err, dvsi.ErrPort) {
		t.Errorf("New with a Read past the buffer = %v", err)
	}
	if _, err := dvsi.New(&brokenPort{readN: -1, writeAll: true}, nil); !errors.Is(err, dvsi.ErrPort) {
		t.Errorf("New with a negative Read = %v", err)
	}
	if _, err := dvsi.New(&brokenPort{writeAll: true}, &dvsi.Config{Timeout: 5 * time.Millisecond}); !errors.Is(err, dvsi.ErrTimeout) {
		t.Errorf("New with a silent Port = %v", err)
	}
	if _, err := dvsi.New(&brokenPort{}, &dvsi.Config{Channel: 3}); !errors.Is(err, dvsi.ErrChannel) {
		t.Errorf("New on channel 3 = %v", err)
	}
	if _, err := dvsi.New(nil, nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("New with a nil Port = %v", err)
	}
	checkSim(t, dp.Device)
}

func TestCodecDeviceErrors(t *testing.T) {
	t.Parallel()

	d := dvsisim.New(dvsisim.Pairs(1), dvsisim.Options{})
	if _, err := dvsi.New(d, &dvsi.Config{Channel: 1, Timeout: 10 * time.Millisecond}); !errors.Is(err, dvsi.ErrTimeout) {
		t.Errorf("New on channel 1 of an AMBE-3000R = %v", err)
	}
	if len(d.Errs) != 1 {
		t.Errorf("device errors %q, want one for channel 1", d.Errs)
	}
	d = dvsisim.New(dvsisim.Pairs(1), dvsisim.Options{})
	d.Reject = 1
	if _, err := dvsi.New(d, nil); !errors.Is(err, dvsi.ErrStatus) {
		t.Errorf("New with a rejected RATEP field = %v", err)
	}
	checkSim(t, d)
}
