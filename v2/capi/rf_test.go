package main

import (
	"encoding/binary"
	"errors"
	"hash/fnv"
	"math"
	"os"
	"regexp"
	"strconv"
	"testing"
	"unsafe"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

func rfBursts(t *testing.T, n int) []rf.Burst {
	t.Helper()
	csbk, err := pdu.NewBSOutboundActivation(0x123456, 3191868)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := layer2.BuildBurst(&csbk, 5, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	var c bs.CACH
	var slc pdu.ShortLC
	out := make([]rf.Burst, n)
	for i := range out {
		tc := enums.Slot1
		if i%2 == 1 {
			tc = enums.Slot2
		}
		cach, err := c.Next(tc, true, &slc)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = rf.Burst{Data: raw, CACH: cach, HasCACH: true, BS: true, Stream: 1, Slot: int64(i)}
	}
	return out
}

func cDefine(t *testing.T, src []byte, name string) uint64 {
	t.Helper()
	m := regexp.MustCompile(`(?m)^#define ` + name + ` (?:UINT64_C\()?(0x[0-9a-f]+|[0-9]+)`).FindSubmatch(src)
	if m == nil {
		t.Fatalf("test/api_test.c has no %s", name)
	}
	v, err := strconv.ParseUint(string(m[1]), 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRFOutputMatchesC(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("test/api_test.c")
	if err != nil {
		t.Fatal(err)
	}
	slots := int(cDefine(t, src, "RF_SLOTS"))
	bursts := rfBursts(t, slots)

	tx, err := rf.NewTransmitter(&rf.TxConfig{Tier: rf.Tier0, Rate: 96000, Region: rf.RegionUS, Service: rf.ServiceLandMobile, Power: 40})
	if re := (*rf.RegionError)(nil); errors.As(err, &re) {
		t.Skip("the region limits refuse the setup:", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	iq := tx.AppendFlush(tx.AppendSamples(nil, bursts))
	h := fnv.New64a()
	_, _ = h.Write(iq)
	if n, want := uint64(len(iq)), cDefine(t, src, "RF_TX_BYTES"); n != want {
		t.Errorf("transmitter: %d bytes, test/api_test.c expects %d", n, want)
	}
	if d, want := h.Sum64(), cDefine(t, src, "RF_TX_FNV"); d != want {
		t.Errorf("transmitter: digest %#x, test/api_test.c expects %#x", d, want)
	}

	atx, err := rf.NewAudioTransmitter(&rf.AudioTxConfig{Tier: rf.Tier0, Rate: 48000, Region: rf.RegionUS, Service: rf.ServiceLandMobile, Power: 40})
	if err != nil {
		t.Fatal(err)
	}
	pcm := atx.AppendFlush(atx.AppendSamples(nil, bursts))
	h.Reset()
	_, _ = h.Write(unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(pcm))), 2*len(pcm)))
	if n, want := uint64(len(pcm)), cDefine(t, src, "RF_AUDIO_SAMPLES"); n != want {
		t.Errorf("audio transmitter: %d samples, test/api_test.c expects %d", n, want)
	}
	if d, want := h.Sum64(), cDefine(t, src, "RF_AUDIO_FNV"); d != want {
		t.Errorf("audio transmitter: digest %#x, test/api_test.c expects %#x", d, want)
	}
}

func rfRRC(t float64) float64 {
	const a = 0.2
	switch {
	case math.Abs(t) < 1e-9:
		return 1 - a + 4*a/math.Pi
	case math.Abs(math.Abs(t)-1/(4*a)) < 1e-9:
		return a / math.Sqrt2 * ((1+2/math.Pi)*math.Sin(math.Pi/(4*a)) + (1-2/math.Pi)*math.Cos(math.Pi/(4*a)))
	}
	return (math.Sin(math.Pi*t*(1-a)) + 4*a*t*math.Cos(math.Pi*t*(1+a))) / (math.Pi * t * (1 - 16*a*a*t*t))
}

func rfIQ9600(t *testing.T) []byte {
	t.Helper()
	const lead, taps, sps = 40, 8, 2
	sym := make([]float64, 0, 48*144)
	for _, b := range rfBursts(t, 48) {
		var bits [36]byte
		copy(bits[:], b.CACH[:])
		copy(bits[3:], b.Data[:])
		for j := range 144 {
			sym = append(sym, [4]float64{1, 3, -1, -3}[bits[j/4]>>(6-2*(j%4))&3])
		}
	}
	f := make([]float64, (len(sym)+2*lead)*sps)
	var sum float64
	for k := -taps * sps; k <= taps*sps; k++ {
		sum += rfRRC(float64(k) / sps)
	}
	for i, s := range sym {
		for k := -taps * sps; k <= taps*sps; k++ {
			f[(lead+i)*sps+k] += s * 648 * rfRRC(float64(k)/sps) * sps / sum
		}
	}
	out := make([]byte, 4*len(f))
	var ph float64
	for i, v := range f {
		ph += 2 * math.Pi * v / 9600
		binary.LittleEndian.PutUint16(out[4*i:], uint16(int16(math.Round(12000*math.Cos(ph)))))
		binary.LittleEndian.PutUint16(out[4*i+2:], uint16(int16(math.Round(12000*math.Sin(ph)))))
	}
	return out
}
