package bs_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func newPool(t testing.TB) []layer2.RawBurst {
	t.Helper()
	p := make([]layer2.RawBurst, 0, 64)
	for _, in := range call(t, 1, 9, 3191868, 2, enums.Slot1, bs.Source{}, 0) {
		p = append(p, in.raw)
	}
	p = append(p, wakeup(t, 1))
	for _, name := range []string{"d-sms.bin", "m-sms.bin"} {
		p = append(p, loadBin(t, name)...)
	}
	return p
}

type script struct {
	cfg bs.Config
	ops []op
}

type op struct {
	kind uint8
	at   time.Duration
	slot enums.Slot
	from bs.Source
	raw  layer2.RawBurst
	h    layer2.Hint
}

func parseScript(data []byte, p []layer2.RawBurst) script {
	var s script
	if len(data) < 2 {
		return s
	}
	s.cfg = bs.Config{Mode: bs.Mode(data[0] & 1), CACH: data[0]&2 != 0, Activation: bs.Activation(data[0] >> 2 & 1),
		ColorCode: 1, CallHang: time.Duration(data[0]>>3) * 100 * time.Millisecond}
	if data[1]&0x80 != 0 {
		s.cfg.ColorCode = enums.ColorCode(data[1] & 0xF)
	}
	var now time.Duration
	for data = data[2:]; len(data) >= 3; data = data[3:] {
		now += time.Duration(data[2]) * 5 * time.Millisecond
		o := op{kind: data[0] % 6, at: now, slot: enums.Slot(data[0]>>3&1 + 1), raw: p[int(data[1])%len(p)]}
		if src := data[0] >> 4 & 3; src == 0 {
			o.from.RF = true
		} else {
			o.from.Peer = uint32(src)
		}
		if data[0]&0x40 != 0 {
			o.raw[data[1]%33] ^= 1 << (data[2] % 8)
		}
		o.h = layer2.Hint{Slot: o.slot, SyncLost: data[0]&0x80 != 0 && data[1]&1 != 0, End: layer2.EndHint(data[1] >> 6 % 3)}
		s.ops = append(s.ops, o)
	}
	return s
}

func runScript(t *testing.T, p []layer2.RawBurst, data []byte) {
	t.Helper()
	s := parseScript(data, p)
	r := newRepeater(t, &s.cfg)
	out := make([]bs.Output, 0, bs.MaxOutputs)
	b := layer2.NewBurst()
	for i := range s.ops {
		o := &s.ops[i]
		var err error
		switch o.kind {
		case 0, 1, 2:
			out, err = r.Inbound(o.at, &o.raw, o.h, o.from, out[:0])
		case 3:
			out = r.Tick(o.at, out[:0])
		case 4:
			out, err = r.Lost(o.at, o.slot, o.from, out[:0])
		case 5:
			err = r.Reject(o.slot)
			out = out[:0]
		}
		if err != nil {
			t.Fatalf("op %d: %v", i, err)
		}
		if len(out) > bs.MaxOutputs {
			t.Fatalf("op %d: %d outputs", i, len(out))
		}
		for j := range out {
			checkOutput(t, b, &s.cfg, &out[j])
		}
	}
}

func checkOutput(t *testing.T, b *layer2.Burst, cfg *bs.Config, o *bs.Output) {
	t.Helper()
	if o.Kind != bs.OutputBurst {
		return
	}
	if o.Slot != enums.Slot1 && o.Slot != enums.Slot2 || o.To == 0 {
		t.Fatalf("burst on %v to %v", o.Slot, o.To)
	}
	if o.HasCACH != (cfg.Mode == bs.ModeRF && cfg.CACH && o.To&bs.ToRF != 0) {
		t.Fatalf("HasCACH %v in mode %v", o.HasCACH, cfg.Mode)
	}
	cc := cfg.ColorCode
	err := b.DecodeFromBytesWith(o.Burst, &layer2.DecodeOptions{ColorCode: &cc})
	if b.Kind == layer2.KindVoice && !b.HasEmbeddedSignalling() {
		return
	}
	if err != nil || b.Kind == layer2.KindData && b.SlotType.FEC.Uncorrectable {
		t.Fatalf("output burst: %v, %v", err, b.SlotType)
	}
}

func seeds() [][]byte {
	rfCall := append(make([]byte, 0, 2+40*6), 0x02, 0x01)
	netCall := append(make([]byte, 0, 2+40*6), 0x01, 0x01)
	for i := range 40 {
		rfCall = append(rfCall, 0, byte(i), 12, 3, 0, 6)
		netCall = append(netCall, 0x10, byte(i), 6, 0x28, byte(i), 2)
	}
	return [][]byte{
		rfCall, netCall,
		{0x06, 0x01, 0, 32, 1, 3, 0, 255, 3, 0, 255},
		{0x02, 0x01, 0, 33, 12, 0, 34, 12, 0, 35, 12, 0, 36, 12, 0, 37, 12, 0, 38, 12, 3, 0, 60},
		{0x03, 0x8F, 0x10, 0, 1, 5, 0, 1, 0x50, 1, 12, 4, 0, 1},
	}
}

func FuzzRepeater(f *testing.F) {
	p := newPool(f)
	for _, s := range seeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		runScript(t, p, data)
	})
}

func corpusFiles(t *testing.T, name string) [][]byte {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata/fuzz", name, "*"))
	if err != nil {
		t.Fatal(err)
	}
	out := make([][]byte, 0, len(files))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) != 2 || lines[0] != "go test fuzz v1" || !strings.HasPrefix(lines[1], "[]byte(") {
			t.Fatalf("%s: not a []byte fuzz input", f)
		}
		s, err := strconv.Unquote(strings.TrimSuffix(strings.TrimPrefix(lines[1], "[]byte("), ")"))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out = append(out, []byte(s))
	}
	return out
}
