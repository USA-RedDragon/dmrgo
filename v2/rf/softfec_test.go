package rf

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

const softTestScale = 1024

type softChannel struct {
	rng   *rand.Rand
	sigma float64
}

func newSoftChannel(seed uint64, esn0 float64) *softChannel {
	return &softChannel{
		rng:   rand.New(rand.NewPCG(seed, uint64(esn0*100)+7)),
		sigma: math.Sqrt(1 / (2 * math.Pow(10, esn0/10))),
	}
}

func (c *softChannel) llrs(bits []unpacked.Bit, dst []int16) {
	for i, b := range bits {
		y := 1 - 2*float64(b&1)
		if c.sigma > 0 {
			y += c.sigma * c.rng.NormFloat64()
		}
		dst[i] = int16(max(math.MinInt16, min(math.MaxInt16, math.Round(y*softTestScale))))
	}
}

func softHardBurst(llr *[softBurstBits]int16) layer2.RawBurst {
	var bits [softBurstBits]unpacked.Bit
	for i, v := range llr {
		if v < 0 {
			bits[i] = 1
		}
	}
	return [33]byte(unpacked.Pack(bits[:]))
}

func softSigns(llr *[softBurstBits]int16) *[softBurstBits]int16 {
	var s [softBurstBits]int16
	for i, v := range llr {
		s[i] = 1
		if v < 0 {
			s[i] = -1
		}
	}
	return &s
}

func softFER(t *testing.T, name string, snrs []float64, trials, units int, cols []string,
	run func(ch *softChannel) []int,
) [][]int {
	t.Helper()
	counts := make([][]int, len(snrs))
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s FER, %d frames per SNR\n%-8s", name, trials*units, "Es/N0")
	for _, c := range cols {
		fmt.Fprintf(&sb, " %14s", c)
	}
	sb.WriteString("\n")
	for i, snr := range snrs {
		ch := newSoftChannel(uint64(i)+1, snr)
		counts[i] = make([]int, len(cols))
		for range trials {
			for c, e := range run(ch) {
				counts[i][c] += e
			}
		}
		fmt.Fprintf(&sb, "%5.1f dB", snr)
		for _, e := range counts[i] {
			fmt.Fprintf(&sb, " %14.5f", float64(e)/float64(trials*units))
		}
		sb.WriteString("\n")
	}
	t.Log(sb.String())
	return counts
}

func softCheckFER(t *testing.T, counts [][]int, hard, soft, mid int, ratio float64) {
	t.Helper()
	for i, row := range counts {
		if row[soft] > row[hard] {
			t.Errorf("SNR %d: soft column %d has %d errors, hard column %d has %d", i, soft, row[soft], hard, row[hard])
		}
	}
	if h, s := counts[mid][hard], counts[mid][soft]; h == 0 || float64(s) > ratio*float64(h) {
		t.Errorf("mid SNR: soft column %d has %d errors, want at most %.2f of hard column %d (%d)", soft, s, ratio, hard, h)
	}
}

func softCost(r *layer2.RawBurst, llr []int16, pos []int16) int64 {
	var c int64
	for _, q := range pos {
		if r[q/8]>>(7-q%8)&1 != 0 {
			c += int64(llr[q%softBurstBits])
		}
	}
	return c
}

func softBool(b bool) int {
	if b {
		return 1
	}
	return 0
}

func softRandomFrames(rng *rand.Rand) [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame {
	var f [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	for i := range f {
		for b := range 49 {
			if rng.IntN(2) == 1 {
				f[i][b/8] |= 0x80 >> (b % 8)
			}
		}
	}
	return f
}

func softFrameErrors(got, want *[layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame) int {
	n := 0
	for i := range got {
		if [3]byte(got[i][:3]) != [3]byte(want[i][:3]) {
			n++
		}
	}
	return n
}

func TestSoftAMBE(t *testing.T) {
	t.Parallel()
	d := newSoftDecoder()
	rng := rand.New(rand.NewPCG(1, 2))
	for range 50 {
		frames := softRandomFrames(rng)
		var r layer2.RawBurst
		_ = r.SetAMBEFrames(frames)
		bits := [264]unpacked.Bit(unpacked.Unpack(r[:]))
		var llr [softBurstBits]int16
		newSoftChannel(0, math.Inf(1)).llrs(bits[:], llr[:])
		var out layer2.RawBurst
		got, res := d.decodeAMBE(&llr, &out)
		if got != frames || res.ErrorsCorrected != 0 || out != r {
			t.Fatalf("noiseless decode: got %v, %+v", got, res)
		}
	}
	snrs := []float64{-2, 0, 2, 4}
	counts := softFER(t, "AMBE+2 C0+C1", snrs, 400, 3, []string{"hard", "soft K=1", "soft K=4", "soft K=8"},
		func(ch *softChannel) []int {
			frames := softRandomFrames(ch.rng)
			var r layer2.RawBurst
			_ = r.SetAMBEFrames(frames)
			bits := [264]unpacked.Bit(unpacked.Unpack(r[:]))
			var llr [softBurstBits]int16
			ch.llrs(bits[:], llr[:])
			hb := softHardBurst(&llr)
			hard, _ := hb.AMBEFrames()
			errs := make([]int, 0, 4)
			errs = append(errs, softFrameErrors(&hard, &frames))
			for _, k := range []int{1, 4, 8} {
				d.AMBECandidates = k
				got, _ := d.decodeAMBE(&llr, nil)
				errs = append(errs, softFrameErrors(&got, &frames))
			}
			d.AMBECandidates = 4
			return errs
		})
	softCheckFER(t, counts, 0, 1, 2, 0.7)
	softCheckFER(t, counts, 0, 2, 2, 0.7)
}

func softBPTCPayload(t testing.TB, rng *rand.Rand) [12]byte {
	var p [12]byte
	for i := range 10 {
		p[i] = byte(rng.IntN(256))
	}
	if err := pdu.Seal(&p, enums.DataTypeCSBK); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSoftBPTC(t *testing.T) {
	t.Parallel()
	d := newSoftDecoder()
	check := softBPTCCheck(enums.DataTypeCSBK)
	rng := rand.New(rand.NewPCG(3, 4))
	for _, dt := range []enums.DataType{enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC} {
		lcCheck := softBPTCCheck(dt)
		for i := range 200 {
			lc := pdu.LCBytes(softRandomLC(rng))
			p, err := lc.Codeword(dt)
			if err != nil {
				t.Fatal(err)
			}
			if i%2 == 1 {
				p[rng.IntN(12)] ^= byte(1 + rng.IntN(255))
			}
			if lcCheck(p) != pdu.FullLCParityOK(p, dt) || lcCheck(p) != (i%2 == 0) {
				t.Fatalf("%s check of %x = %v, pdu.FullLCParityOK = %v", dt, p, lcCheck(p), pdu.FullLCParityOK(p, dt))
			}
		}
	}
	for i := range 20 {
		p := softBPTCPayload(t, rng)
		var r layer2.RawBurst
		r.SetBPTCPayload(p)
		res3 := uint8(i & 7)
		_ = r.SetBPTCReserved(res3)
		bits := [264]unpacked.Bit(unpacked.Unpack(r[:]))
		var llr [softBurstBits]int16
		newSoftChannel(0, math.Inf(1)).llrs(bits[:], llr[:])
		for name, dec := range []func(*[softBurstBits]int16, func([12]byte) bool, *layer2.RawBurst) (softBPTCResult, fec.Result){
			d.decodeBPTCOSD, d.decodeBPTCChase,
		} {
			var out layer2.RawBurst
			got, res := dec(&llr, check, &out)
			if got.Payload != p || got.Reserved != res3 || res.ErrorsCorrected != 0 || res.Uncorrectable || out != r {
				t.Fatalf("decoder %d noiseless decode: got %+v, %+v", name, got, res)
			}
		}
	}
	snrs := []float64{-1, 0, 1, 2}
	cols := []string{"hard", "chase p=4", "chase p=6", "chase p=6 crc", "osd1", "osd2", "osd2 crc", "osd3 w24 crc", "ML bound", "osd2 cands"}
	counts := softFER(t, "BPTC(196,96)", snrs, 400, 1, cols, func(ch *softChannel) []int {
		p := softBPTCPayload(t, ch.rng)
		var r layer2.RawBurst
		r.SetBPTCPayload(p)
		bits := [264]unpacked.Bit(unpacked.Unpack(r[:]))
		var llr [softBurstBits]int16
		ch.llrs(bits[:], llr[:])
		hb := softHardBurst(&llr)
		hard, _ := hb.BPTCPayload()
		errs := make([]int, 0, len(cols))
		errs = append(errs, softBool(hard != p))
		for _, c := range []struct {
			chase       bool
			bits, order int
			window      int
			crc         bool
		}{
			{true, 4, 0, 0, false}, {true, 6, 0, 0, false}, {true, 6, 0, 0, true},
			{false, 0, 1, 0, false}, {false, 0, 2, 0, false}, {false, 0, 2, 0, true}, {false, 0, 3, 24, true},
		} {
			d.ChaseBits, d.OSDOrder, d.OSDWindow = c.bits, c.order, c.window
			var ck func([12]byte) bool
			if c.crc {
				ck = check
			}
			var got softBPTCResult
			if c.chase {
				got, _ = d.decodeBPTCChase(&llr, ck, nil)
			} else {
				got, _ = d.decodeBPTCOSD(&llr, ck, nil)
			}
			errs = append(errs, softBool(got.Payload != p))
		}
		d.ChaseBits, d.OSDOrder, d.OSDWindow = 6, 2, 0
		out := hb
		got, _ := d.decodeBPTCOSD(&llr, nil, &out)
		pos := softBPTCCode.code().pos[:softBPTCCode.code().n]
		errs = append(errs, softBool(got.Payload != p && softCost(&out, llr[:], pos) <= softCost(&r, llr[:], pos)), d.osd.list.scored)
		return errs
	})
	softCheckFER(t, counts, 0, 2, 2, 0.7)
	softCheckFER(t, counts, 0, 5, 2, 0.5)
	softCheckFER(t, counts, 0, 6, 2, 0.5)
}

func TestSoftRate34(t *testing.T) {
	t.Parallel()
	d := newSoftDecoder()
	rng := rand.New(rand.NewPCG(5, 6))
	random := func(rng *rand.Rand) [18]byte {
		var p [18]byte
		for i := range p {
			p[i] = byte(rng.IntN(256))
		}
		return p
	}
	for range 50 {
		p := random(rng)
		var r layer2.RawBurst
		r.SetRate34Payload(p)
		bits := [264]unpacked.Bit(unpacked.Unpack(r[:]))
		var llr [softBurstBits]int16
		newSoftChannel(0, math.Inf(1)).llrs(bits[:], llr[:])
		var out layer2.RawBurst
		got, res := d.decodeRate34(&llr, &out)
		if got != p || res.ErrorsCorrected != 0 || res.Uncorrectable || out != r {
			t.Fatalf("noiseless decode: got %x, %+v", got, res)
		}
	}
	var oneBit, twoBit, twoBitLib, twoBitML, total int
	p := random(rng)
	var clean layer2.RawBurst
	clean.SetRate34Payload(p)
	for _, q := range softTrellisTables.pos {
		j := int(q)
		if j%2 != 0 {
			continue
		}
		for v := 1; v < 4; v++ {
			r := clean
			for b := range 2 {
				if v>>(1-b)&1 != 0 {
					r[(j+b)/8] ^= 0x80 >> ((j + b) % 8)
				}
			}
			bits := [264]unpacked.Bit(unpacked.Unpack(r[:]))
			var llr [softBurstBits]int16
			newSoftChannel(0, math.Inf(1)).llrs(bits[:], llr[:])
			lib, _ := r.Rate34Payload()
			ml, _ := d.decodeRate34(&llr, nil)
			total++
			if v != 3 {
				oneBit += softBool(lib != p) + softBool(ml != p)
				continue
			}
			twoBit++
			twoBitLib += softBool(lib != p)
			twoBitML += softBool(ml != p)
		}
	}
	t.Logf("%d single dibit errors: 1-bit errors decoded wrong %d times; of %d 2-bit errors the library decodes %d wrong and hard ML %d",
		total, oneBit, twoBit, twoBitLib, twoBitML)
	if oneBit != 0 {
		t.Errorf("1-bit errors decoded wrong %d times", oneBit)
	}
	cost := func(r *layer2.RawBurst, llr *[softBurstBits]int16) int64 {
		var c int64
		for _, q := range softTrellisTables.pos {
			if r[q/8]>>(7-q%8)&1 != 0 {
				c += int64(llr[q])
			}
		}
		return c
	}
	closer, worse := 0, 0
	ch := newSoftChannel(99, 3)
	for range 2000 {
		p := random(ch.rng)
		var r layer2.RawBurst
		r.SetRate34Payload(p)
		bits := [264]unpacked.Bit(unpacked.Unpack(r[:]))
		var llr [softBurstBits]int16
		ch.llrs(bits[:], llr[:])
		hb := softHardBurst(&llr)
		_, lres := hb.Rate34Payload()
		var out layer2.RawBurst
		_, mres := d.decodeRate34(softSigns(&llr), nil)
		d.decodeRate34(&llr, &out)
		if mres.ErrorsCorrected > lres.ErrorsCorrected {
			worse++
		}
		if mres.ErrorsCorrected < lres.ErrorsCorrected {
			closer++
		}
		if cost(&out, &llr) > cost(&r, &llr) {
			t.Fatalf("soft Viterbi codeword costs more than the transmitted codeword")
		}
	}
	t.Logf("2000 blocks at 3 dB: hard ML path closer to the hard decisions than the library path %d times, farther %d times", closer, worse)
	if worse != 0 {
		t.Errorf("hard ML path farther than the library path %d times", worse)
	}
	snrs := []float64{2, 3, 4, 5}
	counts := softFER(t, "Rate 3/4 trellis", snrs, 500, 1, []string{"hard library", "hard ML", "soft ML"},
		func(ch *softChannel) []int {
			p := random(ch.rng)
			var r layer2.RawBurst
			r.SetRate34Payload(p)
			bits := [264]unpacked.Bit(unpacked.Unpack(r[:]))
			var llr [softBurstBits]int16
			ch.llrs(bits[:], llr[:])
			hb := softHardBurst(&llr)
			lib, _ := hb.Rate34Payload()
			hml, _ := d.decodeRate34(softSigns(&llr), nil)
			sml, _ := d.decodeRate34(&llr, nil)
			return []int{softBool(lib != p), softBool(hml != p), softBool(sml != p)}
		})
	softCheckFER(t, counts, 0, 2, 1, 0.5)
	softCheckFER(t, counts, 1, 2, 1, 0.5)
}

func TestSoftEMBAndSlotType(t *testing.T) {
	t.Parallel()
	d := newSoftDecoder()
	for v := range 256 {
		var r layer2.RawBurst
		st := pdu.SlotTypeInfo(v)
		_ = r.SetSlotType(st.SlotType())
		emb := pdu.EMBInfo(v & 0xfe)
		_ = r.SetEMB(emb.EmbeddedSignalling())
		bits := [264]unpacked.Bit(unpacked.Unpack(r[:]))
		var llr [softBurstBits]int16
		newSoftChannel(0, math.Inf(1)).llrs(bits[:], llr[:])
		var out layer2.RawBurst
		gs, rs := d.decodeSlotType(&llr, 0, false, &out)
		ge, re := d.decodeEMB(&llr, 0, false, &out)
		if gs != st || ge != emb || rs.ErrorsCorrected != 0 || re.ErrorsCorrected != 0 || out != r {
			t.Fatalf("noiseless decode of %#x: slot type %#x, EMB %#x", v, gs, ge)
		}
	}
	snrs := []float64{-1, 1, 3, 5}
	cols := []string{"slot hard", "slot soft", "slot soft cc", "emb hard", "emb soft", "emb soft cc"}
	counts := softFER(t, "Slot type Golay(20,8) and EMB QR(16,7)", snrs, 3000, 1, cols, func(ch *softChannel) []int {
		st := pdu.SlotTypeInfo(ch.rng.IntN(256))
		emb := pdu.EMBInfo(ch.rng.IntN(128) << 1)
		var r layer2.RawBurst
		_ = r.SetSlotType(st.SlotType())
		_ = r.SetEMB(emb.EmbeddedSignalling())
		bits := [264]unpacked.Bit(unpacked.Unpack(r[:]))
		var llr [softBurstBits]int16
		ch.llrs(bits[:], llr[:])
		hb := softHardBurst(&llr)
		hst := hb.SlotType()
		hemb := hb.EMB()
		s1, _ := d.decodeSlotType(&llr, 0, false, nil)
		s2, _ := d.decodeSlotType(&llr, st.ColorCode(), true, nil)
		e1, _ := d.decodeEMB(&llr, 0, false, nil)
		e2, _ := d.decodeEMB(&llr, emb.ColorCode(), true, nil)
		hstInfo, _ := hst.Info()
		hembInfo, _ := hemb.Info()
		return []int{
			softBool(hstInfo != st), softBool(s1 != st), softBool(s2 != st),
			softBool(hembInfo != emb), softBool(e1 != emb), softBool(e2 != emb),
		}
	})
	softCheckFER(t, counts, 0, 1, 1, 0.7)
	softCheckFER(t, counts, 1, 2, 1, 1)
	softCheckFER(t, counts, 3, 4, 1, 0.7)
	softCheckFER(t, counts, 4, 5, 1, 1)
}

func softCACH(t testing.TB, rng *rand.Rand, info [softShortLCInfo]unpacked.Bit) ([4][layer2.CACHBits]unpacked.Bit, [4]pdu.TACT) {
	var out [4][layer2.CACHBits]unpacked.Bit
	var tacts [4]pdu.TACT
	for i, f := range bptc.EncodeCACH(unpacked.Uint(info[:])) {
		tacts[i] = softTACT(uint32(rng.IntN(16)))
		c := layer2.CACH{TACT: tacts[i]}
		c.Payload = cachPayload(f)
		logical, err := c.Encode()
		if err != nil {
			t.Fatal(err)
		}
		out[i] = cachBits(layer2.CACHInterleave(logical))
	}
	return out, tacts
}

func softShortLCInfoBits(rng *rand.Rand) [softShortLCInfo]unpacked.Bit {
	var info [softShortLCInfo]unpacked.Bit
	for i := range softShortLCData {
		info[i] = unpacked.Bit(rng.IntN(2))
	}
	unpacked.PutUint(info[softShortLCData:], uint64(crc.CRC8(unpacked.Pack(info[:softShortLCData]), softShortLCData)))
	return info
}

func softSameTACT(a, b pdu.TACT) bool {
	return a.AccessType == b.AccessType && a.TDMAChannel == b.TDMAChannel && a.LCSS == b.LCSS
}

func TestSoftCACH(t *testing.T) {
	t.Parallel()
	d := newSoftDecoder()
	rng := rand.New(rand.NewPCG(7, 8))
	for range 50 {
		info := softShortLCInfoBits(rng)
		cach, tacts := softCACH(t, rng, info)
		var llr [4][layer2.CACHBits]int16
		for i := range cach {
			newSoftChannel(0, math.Inf(1)).llrs(cach[i][:], llr[i][:])
		}
		var out [4][layer2.CACHBits]unpacked.Bit
		got, res := d.decodeShortLC(&llr, &out)
		for i := range tacts {
			tact, tres := d.decodeTACT(&llr[i], &out[i])
			if !softSameTACT(tact, tacts[i]) || tres.ErrorsCorrected != 0 {
				t.Fatalf("noiseless TACT decode: got %+v, want %+v", tact, tacts[i])
			}
		}
		if got != info || res.ErrorsCorrected != 0 || res.Uncorrectable || out != cach {
			t.Fatalf("noiseless Short LC decode: got %v, %+v", got, res)
		}
	}
	snrs := []float64{-1, 1, 3, 5}
	lists := []int{1, 2, 4, 8}
	cols := make([]string, 0, 4+2*len(lists))
	cols = append(cols, "tact hard", "tact soft", "slc hard", "hard undet")
	for _, k := range lists {
		cols = append(cols, fmt.Sprintf("K=%d", k), fmt.Sprintf("K=%d undet", k))
	}
	counts := softFER(t, "TACT Hamming(7,4) and Short LC BPTC(68,36)", snrs, 1000, 1, cols, func(ch *softChannel) []int {
		info := softShortLCInfoBits(ch.rng)
		cach, tacts := softCACH(t, ch.rng, info)
		var llr [4][layer2.CACHBits]int16
		var frags [4][3]byte
		var hardTACT pdu.TACT
		for i := range cach {
			ch.llrs(cach[i][:], llr[i][:])
			var hard [layer2.CACHBits]unpacked.Bit
			for j, v := range llr[i] {
				hard[j] = unpacked.Bit(softBool(v < 0))
			}
			c := layer2.DecodeCACHField(cachBytes(hard))
			frags[i] = c.Payload
			if i == 0 {
				hardTACT = c.TACT
			}
		}
		var words [4]uint32
		for i := range frags {
			words[i] = cachPayloadWord(frags[i])
		}
		hv, _ := bptc.DecodeCACH(words)
		var hinfo [softShortLCInfo]unpacked.Bit
		unpacked.PutUint(hinfo[:], hv)
		hardOK := crc.CheckCRC8(unpacked.Pack(hinfo[:softShortLCData]), softShortLCData, uint8(unpacked.Uint(hinfo[softShortLCData:])))
		tact, _ := d.decodeTACT(&llr[0], nil)
		errs := make([]int, 0, len(cols))
		errs = append(errs, softBool(!softSameTACT(hardTACT, tacts[0])), softBool(!softSameTACT(tact, tacts[0])),
			softBool(hinfo != info), softBool(hinfo != info && hardOK))
		for _, k := range lists {
			d.ShortLCList = k
			sinfo, res := d.decodeShortLC(&llr, nil)
			errs = append(errs, softBool(sinfo != info), softBool(sinfo != info && !res.Uncorrectable))
		}
		d.ShortLCList = 4
		return errs
	})
	softCheckFER(t, counts, 0, 1, 1, 0.8)
	for c := range lists {
		softCheckFER(t, counts, 2, 4+2*c, 1, 0.5)
	}
}

func softEmbBursts(lc [9]byte) [4]layer2.RawBurst {
	var r [4]layer2.RawBurst
	for i, f := range layer2.EncodeEmbeddedLC(lc) {
		r[i].SetEmbeddedData(f)
	}
	return r
}

func softRandomLC(rng *rand.Rand) [9]byte {
	var lc [9]byte
	for i := range lc {
		lc[i] = byte(rng.IntN(256))
	}
	return lc
}

func TestSoftEmbeddedLC(t *testing.T) {
	t.Parallel()
	d := newSoftDecoder()
	rng := rand.New(rand.NewPCG(9, 10))
	for range 50 {
		lc := softRandomLC(rng)
		bursts := softEmbBursts(lc)
		var llr [4][softBurstBits]int16
		for i := range bursts {
			bits := [264]unpacked.Bit(unpacked.Unpack(bursts[i][:]))
			newSoftChannel(0, math.Inf(1)).llrs(bits[:], llr[i][:])
		}
		var out [4]layer2.RawBurst
		got, res := d.decodeEmbeddedLC(&llr, &out)
		if got != lc || res.ErrorsCorrected != 0 || res.Uncorrectable || out != bursts {
			t.Fatalf("noiseless decode: got %x, %+v", got, res)
		}
	}
	snrs := []float64{-2, -1, 0, 1}
	lists := []int{1, 2, 4, 8}
	cols := make([]string, 0, 4+2*len(lists))
	cols = append(cols, "hard", "hard undet")
	for _, k := range lists {
		cols = append(cols, fmt.Sprintf("K=%d", k), fmt.Sprintf("K=%d undet", k))
	}
	cols = append(cols, "hard 2 copies", "K=2 2 copies")
	counts := softFER(t, "Embedded LC BPTC(128,77)", snrs, 500, 1, cols, func(ch *softChannel) []int {
		lc := softRandomLC(ch.rng)
		bursts := softEmbBursts(lc)
		var llr, llr2 [4][softBurstBits]int16
		errs := make([]int, 0, len(cols))
		for copies := 1; copies <= 2; copies++ {
			for i := range bursts {
				bits := [264]unpacked.Bit(unpacked.Unpack(bursts[i][:]))
				ch.llrs(bits[:], llr2[i][:])
				if copies == 1 {
					llr[i] = llr2[i]
				} else {
					softCombine(llr[i][:], llr2[i][:])
				}
			}
			var frags [4][4]byte
			for i := range llr {
				hb := softHardBurst(&llr[i])
				frags[i] = hb.EmbeddedData()
			}
			hard, hres := layer2.DecodeEmbeddedLC(frags)
			if copies == 2 {
				soft, _ := d.decodeEmbeddedLC(&llr, nil)
				errs = append(errs, softBool(hard != lc), softBool(soft != lc))
				break
			}
			errs = append(errs, softBool(hard != lc), softBool(hard != lc && !hres.Uncorrectable))
			for _, k := range lists {
				d.EmbLCList = k
				soft, res := d.decodeEmbeddedLC(&llr, nil)
				errs = append(errs, softBool(soft != lc), softBool(soft != lc && !res.Uncorrectable))
			}
			d.EmbLCList = 2
		}
		return errs
	})
	for c := range lists {
		softCheckFER(t, counts, 0, 2+2*c, 1, 0.5)
	}
	softCheckFER(t, counts, len(cols)-2, len(cols)-1, 1, 1)
}

func TestSoftCombine(t *testing.T) {
	t.Parallel()
	dst := []int16{30000, -30000, 5, -5}
	softCombine(dst, []int16{10000, -10000, -10, 10})
	if want := []int16{math.MaxInt16, math.MinInt16, -5, 5}; fmt.Sprint(dst) != fmt.Sprint(want) {
		t.Errorf("softCombine = %v, want %v", dst, want)
	}
}

type softBenchInputs struct {
	voice, bptc, rate34, emb [softBurstBits]int16
	cach                     [4][layer2.CACHBits]int16
	lc                       [4][softBurstBits]int16
}

func newSoftBenchInputs(tb testing.TB, esn0 float64) *softBenchInputs {
	tb.Helper()
	ch := newSoftChannel(11, esn0)
	in := &softBenchInputs{}
	var r layer2.RawBurst
	_ = r.SetAMBEFrames(softRandomFrames(ch.rng))
	bits := [264]unpacked.Bit(unpacked.Unpack(r[:]))
	ch.llrs(bits[:], in.voice[:])
	r = layer2.RawBurst{}
	r.SetBPTCPayload(softBPTCPayload(tb, ch.rng))
	_ = r.SetSlotType(pdu.SlotType{ColorCode: 1, DataType: enums.DataTypeCSBK})
	bits = [264]unpacked.Bit(unpacked.Unpack(r[:]))
	ch.llrs(bits[:], in.bptc[:])
	r = layer2.RawBurst{}
	r.SetRate34Payload([18]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18})
	bits = [264]unpacked.Bit(unpacked.Unpack(r[:]))
	ch.llrs(bits[:], in.rate34[:])
	r = layer2.RawBurst{}
	_ = r.SetEMB(pdu.EMBInfo(0x12).EmbeddedSignalling())
	bits = [264]unpacked.Bit(unpacked.Unpack(r[:]))
	ch.llrs(bits[:], in.emb[:])
	cach, _ := softCACH(tb, ch.rng, softShortLCInfoBits(ch.rng))
	for i := range cach {
		ch.llrs(cach[i][:], in.cach[i][:])
	}
	for i, b := range softEmbBursts(softRandomLC(ch.rng)) {
		bits = [264]unpacked.Bit(unpacked.Unpack(b[:]))
		ch.llrs(bits[:], in.lc[i][:])
	}
	return in
}

func softBenchCases(d *softDecoder, in *softBenchInputs) []struct {
	name string
	run  func()
} {
	check := softBPTCCheck(enums.DataTypeCSBK)
	lcCheck := softBPTCCheck(enums.DataTypeVoiceLCHeader)
	var out layer2.RawBurst
	var outs [4]layer2.RawBurst
	var cout [4][layer2.CACHBits]unpacked.Bit
	return []struct {
		name string
		run  func()
	}{
		{"AMBE K=1", func() { d.AMBECandidates = 1; d.decodeAMBE(&in.voice, &out) }},
		{"AMBE K=4", func() { d.AMBECandidates = 4; d.decodeAMBE(&in.voice, &out) }},
		{"BPTC chase p=4", func() { d.ChaseBits = 4; d.decodeBPTCChase(&in.bptc, check, &out) }},
		{"BPTC chase p=6", func() { d.ChaseBits = 6; d.decodeBPTCChase(&in.bptc, check, &out) }},
		{"BPTC osd2", func() { d.OSDOrder, d.OSDWindow = 2, 0; d.decodeBPTCOSD(&in.bptc, check, &out) }},
		{"BPTC osd2 LC check", func() { d.OSDOrder, d.OSDWindow = 2, 0; d.decodeBPTCOSD(&in.bptc, lcCheck, &out) }},
		{"BPTC osd2 no check", func() { d.OSDOrder, d.OSDWindow = 2, 0; d.decodeBPTCOSD(&in.bptc, nil, &out) }},
		{"BPTC osd3 w24", func() { d.OSDOrder, d.OSDWindow = 3, 24; d.decodeBPTCOSD(&in.bptc, check, &out) }},
		{"BPTC grid", func() { d.OSDOrder, d.OSDWindow = 2, 0; d.decodeBPTCGrid(&in.bptc, check, &out) }},
		{"Rate34 viterbi", func() { d.decodeRate34(&in.rate34, &out) }},
		{"SlotType", func() { d.decodeSlotType(&in.bptc, 0, false, &out) }},
		{"SlotType cc", func() { d.decodeSlotType(&in.bptc, 1, true, &out) }},
		{"EMB", func() { d.decodeEMB(&in.emb, 0, false, &out) }},
		{"EMB cc", func() { d.decodeEMB(&in.emb, 1, true, &out) }},
		{"TACT", func() { d.decodeTACT(&in.cach[0], &cout[0]) }},
		{"ShortLC osd2", func() { d.OSDOrder, d.OSDWindow = 2, 0; d.decodeShortLC(&in.cach, &cout) }},
		{"EmbeddedLC osd2", func() { d.OSDOrder, d.OSDWindow = 2, 0; d.decodeEmbeddedLC(&in.lc, &outs) }},
		{"combine 264", func() { softCombine(in.voice[:], in.bptc[:]) }},
	}
}

func BenchmarkSoftFEC(b *testing.B) {
	d := newSoftDecoder()
	in := newSoftBenchInputs(b, 3)
	for _, c := range softBenchCases(d, in) {
		b.Run(strings.ReplaceAll(c.name, " ", "_"), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				c.run()
			}
		})
	}
}

func TestSoftAMBEScramble(t *testing.T) {
	t.Parallel()
	c := softAMBESlot(0).c1
	enc := softAMBEEncoder(0, 0)
	for v := range uint32(1 << softAMBEC0) {
		img, err := enc(v)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := softAMBEPN(v), softMask(img, c.pos[:c.n]); got != want {
			t.Fatalf("C0 %#x: mask %#x, want %#x", v, got, want)
		}
	}
}

func TestSoftOSDReprocess(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(5, 6))
	ref, nib, byt := newSoftDecoder(), newSoftDecoder(), newSoftDecoder()
	byt.setBytes(&options{OSDBytes: true})
	for _, id := range []softOSDID{softBPTCCode, softShortLCCode, softEmbLCCode} {
		c := &softOSDCodes[id]
		for trial := range 60 {
			var cw [softOSDWords]uint64
			for i := range c.k {
				if rng.IntN(2) == 1 {
					for x := range cw {
						cw[x] ^= c.rows[i][x]
					}
				}
			}
			sigma := 0.6 + 0.02*float64(trial)
			for j := range c.n {
				v := 1.0
				if softBit(&cw, j) {
					v = -1
				}
				l := int32((v + rng.NormFloat64()*sigma) * 40)
				ref.osd.l[j], nib.osd.l[j], byt.osd.l[j] = l, l, l
			}
			order, window := 1+trial%3, []int{0, 12, 40}[trial%3]
			ref.osd.accept, nib.osd.accept, byt.osd.accept = nil, nil, nil
			id.refOSD(&ref.osd, order, window, softListMax)
			for _, d := range []*softDecoder{nib, byt} {
				id.osd(&d.osd, order, window, softListMax)
				a, b := &ref.osd.list, &d.osd.list
				if a.n != b.n || a.scored != b.scored || a.cost != b.cost || a.cw != b.cw {
					t.Fatalf("code %d trial %d bytes %v: lists differ (n %d %d, scored %d %d)", id, trial, d.osd.btab != nil, a.n, b.n, a.scored, b.scored)
				}
			}
		}
	}
}

func (id softOSDID) refOSD(s *softOSDScratch, order, window, listMax int) {
	c := &softOSDCodes[id]
	k := c.k
	s.prepare(c.n)
	softNibbleTables(s.tab, s.mag[:c.n])
	id.basis(s)
	s.nw = (c.n + 63) / 64
	var cand [4][softOSDWords]uint64
	c0, ci, cj, cw := &cand[0], &cand[1], &cand[2], &cand[3]
	for i := range k {
		if j := s.pivot[i]; s.hard[j>>6]>>(j&63)&1 != 0 {
			s.flip(c0, c0, i)
		}
	}
	s.list.reset(listMax)
	s.pass, s.floor = math.MaxInt32, 0
	for j := range c.n {
		s.floor += min(0, s.l[j])
	}
	s.refTry(c0)
	m := &s.pmag
	for i := range k {
		m[i] = s.mag[s.pivot[i]]
	}
	for i := k - 1; order >= 1 && i >= 0 && m[i] < s.thr(); i-- {
		s.flip(ci, c0, i)
		s.refTry(ci)
	}
	for j := k - 1; order >= 2 && j >= 1 && m[j]+m[j-1] < s.thr(); j-- {
		s.flip(cj, c0, j)
		for i := j - 1; i >= 0 && m[i]+m[j] < s.thr(); i-- {
			s.flip(ci, cj, i)
			s.refTry(ci)
		}
	}
	lo := max(0, k-window)
	for a := k - 1; order >= 3 && a >= lo+2 && m[a]+m[a-1]+m[a-2] < s.thr(); a-- {
		s.flip(cj, c0, a)
		for b := a - 1; b >= lo+1 && m[a]+m[b]+m[b-1] < s.thr(); b-- {
			s.flip(ci, cj, b)
			for x := b - 1; x >= lo && m[a]+m[b]+m[x] < s.thr(); x-- {
				s.flip(cw, ci, x)
				s.refTry(cw)
			}
		}
	}
}

func (s *softOSDScratch) refTry(cw *[softOSDWords]uint64) {
	bound := int32(math.MaxInt32)
	if s.list.n == s.list.max {
		bound = s.list.cost[s.list.n-1]
	}
	cost := s.floor
	for x := range s.nw {
		t := s.tab[256*x:]
		for i, e := 0, cw[x]^s.hard[x]; e != 0; i, e = i+16, e>>4 {
			cost += t[i+int(e&0xf)]
		}
		if cost >= bound {
			break
		}
	}
	if s.list.add(cost, cw) && s.accept != nil && cost < s.pass && s.accept(cw) {
		s.pass = cost
	}
}

func TestSoftOSDCleanWords(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(7, 8))
	d := newSoftDecoder()
	noisy := func(id softOSDID) {
		for j := range softOSDCodes[id].n {
			d.osd.l[j] = int32((1 + rng.NormFloat64()) * 40)
		}
		d.osd.accept = nil
		id.osd(&d.osd, 2, 0, softListMax)
	}
	for range 4 {
		noisy(softBPTCCode)
		for _, id := range []softOSDID{softShortLCCode, softEmbLCCode} {
			noisy(id)
			for i := range d.osd.list.n {
				if cw := &d.osd.list.cw[i]; cw[2] != 0 || cw[3] != 0 {
					t.Fatalf("code %d candidate %d: bits past the code %x", id, i, *cw)
				}
			}
		}
	}
}

func (d *softDecoder) decodeEmbeddedLC(llr *[4][softBurstBits]int16, out *[4]layer2.RawBurst) ([9]byte, fec.Result) {
	var frags [4][embBits]int16
	for i := range frags {
		copy(frags[i][:], llr[i][embFirstBit:embFirstBit+embBits])
	}
	return d.decodeEmbeddedFrags(&frags, out)
}
