package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/cmplx"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

const (
	weakPath  = "../../../cmd/dmrrf/testdata/xpr8400_bs_voice_weak.cu8"
	sweepPath = "../../../cmd/dmrrf/testdata/xpr8400_gain_sweep.cu8"
	fixRate   = 256000
	wantLC    = `"src":3191868,"dst":9`
)

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func decodeAll(t *testing.T, opt options, data []byte, chunk int) ([]rf.Burst, string) {
	t.Helper()
	d, err := newDecoder(opt)
	if err != nil {
		t.Fatal(err)
	}
	var got []rf.Burst
	d.onBurst = func(b *rf.Burst) { got = append(got, *b) }
	var out bytes.Buffer
	for len(data) > 0 {
		n := min(chunk, len(data))
		if err := d.write(data[:n]); err != nil {
			t.Fatal(err)
		}
		out.Write(d.rep.take())
		data = data[n:]
	}
	if err := d.flush(); err != nil {
		t.Fatal(err)
	}
	out.Write(d.rep.take())
	return got, out.String()
}

func nativeIQ(t *testing.T, data []byte, tier rf.Tier) []rf.Burst {
	t.Helper()
	rx, err := rf.New(&rf.Config{Rate: fixRate, Tier: tier})
	if err != nil {
		t.Fatal(err)
	}
	var got []rf.Burst
	for len(data) > 0 {
		n := min(1<<16, len(data))
		bs, err := rx.Write(data[:n])
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, bs...)
		data = data[n:]
	}
	return append(got, rx.Flush()...)
}

func nativeDisc(t *testing.T, pcm []int16, hz int, tier rf.Tier) []rf.Burst {
	t.Helper()
	rx, err := rf.NewDisc(&rf.DiscConfig{Hz: hz, Tier: tier})
	if err != nil {
		t.Fatal(err)
	}
	var got []rf.Burst
	for len(pcm) > 0 {
		n := min(1<<12, len(pcm))
		bs, err := rx.Write(pcm[:n])
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, bs...)
		pcm = pcm[n:]
	}
	return append(got, rx.Flush()...)
}

func sameBursts(t *testing.T, got, want []rf.Burst) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d bursts, native receiver %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("burst %d differs from the native receiver:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

func TestIQMatchesNative(t *testing.T) {
	t.Parallel()
	for _, path := range []string{weakPath, sweepPath} {
		data := readFixture(t, path)
		for _, tier := range []int{1, 2} {
			want := nativeIQ(t, data, rf.Tier0+rf.Tier(tier))
			got, out := decodeAll(t, options{format: "cu8", rate: fixRate, audioTier: -1, iqTier: tier}, data, 777)
			sameBursts(t, got, want)
			if len(want) < 15 {
				t.Errorf("%s tier %d: %d bursts", path, tier, len(want))
			}
			if path == weakPath && !strings.Contains(out, wantLC) {
				t.Errorf("%s tier %d: no LC with %s in the output", path, tier, wantLC)
			}
		}
	}
}

func toCS16(cu8 []byte) []byte {
	out := make([]byte, 2*len(cu8))
	for i, v := range cu8 {
		binary.LittleEndian.PutUint16(out[2*i:], uint16(int16(math.Round((float64(v)-127.5)*256))))
	}
	return out
}

func burstSet(bs []rf.Burst) map[[33]byte]bool {
	m := map[[33]byte]bool{}
	for i := range bs {
		d, _ := bs[i].Decoded()
		m[d] = true
	}
	return m
}

func TestCS16(t *testing.T) {
	t.Parallel()
	data := readFixture(t, weakPath)
	want := burstSet(nativeIQ(t, data, rf.Tier2))
	got, out := decodeAll(t, options{format: "cs16", rate: fixRate, audioTier: -1, iqTier: 2}, toCS16(data), 1001)
	if !strings.Contains(out, wantLC) {
		t.Errorf("no LC with %s in the output", wantLC)
	}
	same := 0
	for d := range burstSet(got) {
		if want[d] {
			same++
		}
	}
	if same < len(want)*9/10 {
		t.Errorf("%d of %d bursts of the cu8 input decoded the same from cs16", same, len(want))
	}
}

func discriminate(t *testing.T, cu8 []byte, hz float64) []float32 {
	t.Helper()
	rs := newResampler(fixRate, hz, true)
	var bb []complex64
	for i := 0; i+1 < len(cu8); i += 2 {
		bb = rs.push(complex(normalize(encU8, cu8[i:]), normalize(encU8, cu8[i+1:])), bb)
	}
	out := make([]float32, len(bb))
	for i := 1; i < len(bb); i++ {
		out[i] = float32(cmplx.Phase(complex128(bb[i]*complex(real(bb[i-1]), -imag(bb[i-1])))) / math.Pi)
	}
	return out
}

func wavFile(rate int, enc encoding, x []float32) []byte {
	sz := enc.size()
	tag := uint16(1)
	if enc == encF32 {
		tag = 3
	}
	le := binary.LittleEndian
	b := []byte("RIFF")
	b = le.AppendUint32(b, uint32(48+len(x)*sz))
	b = append(b, "WAVEfmt "...)
	b = le.AppendUint32(b, 16)
	b = le.AppendUint16(b, tag)
	b = le.AppendUint16(b, 1)
	b = le.AppendUint32(b, uint32(rate))
	b = le.AppendUint32(b, uint32(rate*sz))
	b = le.AppendUint16(b, uint16(sz))
	b = le.AppendUint16(b, uint16(8*sz))
	b = append(b, "LIST"...)
	b = le.AppendUint32(b, 4)
	b = append(b, "INFOdata"...)
	b = le.AppendUint32(b, uint32(len(x)*sz))
	for _, v := range x {
		if enc == encF32 {
			b = le.AppendUint32(b, math.Float32bits(v))
			continue
		}
		b = le.AppendUint16(b, uint16(int16(math.Round(float64(max(-1, min(1, v))*32767)))))
	}
	return b
}

func TestAudioMatchesNative(t *testing.T) {
	t.Parallel()
	audio := discriminate(t, readFixture(t, weakPath), audioHz)
	pcm := make([]int16, len(audio))
	for i, v := range audio {
		pcm[i] = int16(math.Round(float64(max(-1, min(1, v)) * 32767)))
	}
	for _, tier := range []int{0, 1, 2} {
		want := nativeDisc(t, pcm, audioHz, rf.Tier0+rf.Tier(tier))
		got, out := decodeAll(t, options{format: "wav", audioTier: tier, iqTier: -1}, wavFile(audioHz, encS16, audio), 333)
		sameBursts(t, got, want)
		if len(want) < 15 {
			t.Errorf("tier %d: %d bursts", tier, len(want))
		}
		if !strings.Contains(out, wantLC) {
			t.Errorf("tier %d: no LC with %s in the output", tier, wantLC)
		}
	}
}

func TestAudio48k(t *testing.T) {
	t.Parallel()
	audio := discriminate(t, readFixture(t, weakPath), 48000)
	got, out := decodeAll(t, options{format: "wav", audioTier: 1, iqTier: -1}, wavFile(48000, encF32, audio), 4096)
	if len(got) < 15 || !strings.Contains(out, wantLC) {
		t.Errorf("%d bursts from 48 kHz float audio, LC with %s found %v", len(got), wantLC, strings.Contains(out, wantLC))
	}
	raw := make([]byte, 4*len(audio))
	for i, v := range audio {
		binary.LittleEndian.PutUint32(raw[4*i:], math.Float32bits(v))
	}
	got2, _ := decodeAll(t, options{format: "f32", rate: 48000, audioTier: 1, iqTier: -1}, raw, 4*128)
	sameBursts(t, got2, got)
}
