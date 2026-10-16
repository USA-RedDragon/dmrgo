// Package outputdigest_test pins the decode and encode output of the library
// for many random and corrupted inputs, as one SHA-256 digest per category,
// so that a change of internals that changes any output fails.
package outputdigest_test

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"hash"
	"math/rand"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

var update = flag.Bool("update", false, "rewrite testdata/digests.txt")

const goldenFile = "testdata/digests.txt"

type digests map[string]hash.Hash

func (d digests) add(cat string, format string, args ...any) {
	h, ok := d[cat]
	if !ok {
		h = sha256.New()
		d[cat] = h
	}
	fmt.Fprintf(h, format, args...)
	h.Write([]byte{'\n'})
}

func randBits(r *rand.Rand, dst []bit.Bit) {
	for i := range dst {
		dst[i] = bit.Bit(r.Intn(2))
	}
}

// corrupt flips up to max random bits of b.
func corrupt(r *rand.Rand, b []byte, maxFlips int) {
	for range r.Intn(maxFlips + 1) {
		i := r.Intn(len(b) * 8)
		b[i/8] ^= 0x80 >> (i % 8)
	}
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

func dataString(d layer2.Data) string {
	if d == nil {
		return "<nil>"
	}
	bits, err := encodeBits(d)
	return fmt.Sprintf("%T %s enc=%x err=%s", d, d.String(), bit.Pack(bits), errString(err))
}

// encodeBits returns the bits of the Encode method of d as a slice, or nil
// and the error of Encode.
func encodeBits(d layer2.Data) ([]bit.Bit, error) {
	var bits []bit.Bit
	var err error
	switch e := d.(type) {
	case interface{ Encode() ([96]bit.Bit, error) }:
		var a [96]bit.Bit
		a, err = e.Encode()
		bits = a[:]
	case interface{ Encode() ([144]bit.Bit, error) }:
		var a [144]bit.Bit
		a, err = e.Encode()
		bits = a[:]
	case interface{ Encode() ([192]bit.Bit, error) }:
		var a [192]bit.Bit
		a, err = e.Encode()
		bits = a[:]
	default:
		return nil, fmt.Errorf("%T has no Encode method", d)
	}
	if err != nil {
		return nil, err
	}
	return bits, nil
}

func burstString(b *layer2.Burst) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "k=%v sp=%v ns=%v sd=%d vb=%v vs=%v st=%+v rsv=%v emb=%+v ed=%v rc=%+v fec=%+v hes=%v hrc=%v ",
		b.Kind, b.SyncPattern, b.NearestSync, b.SyncDistance, b.VoiceBurst, b.VoiceSlots, b.SlotType, b.BPTCReserved,
		b.EmbeddedSignalling, b.EmbeddedSignallingData, b.ReverseChannel, b.FEC, b.HasEmbeddedSignalling(), b.HasReverseChannel())
	sb.WriteString(dataString(b.Data))
	out, err := b.Encode()
	fmt.Fprintf(&sb, " out=%x err=%s", out, errString(err))
	frames, res := b.AMBEFrames()
	fmt.Fprintf(&sb, " ambe=%v %+v", frames, res)
	return sb.String()
}

func rawBurstString(rb layer2.RawBurst) string {
	var sb strings.Builder
	sync, d := rb.Sync()
	fmt.Fprintf(&sb, "sync=%v/%d st=%+v emb=%+v ed=%x rcemb=%+v rcd=%x vs=%x ", sync, d, rb.SlotType(), rb.EMB(), rb.EmbeddedData(),
		rb.StandaloneRCEMB(), rb.StandaloneRCData(), rb.VoiceSlots())
	p, res := rb.BPTCPayload()
	fmt.Fprintf(&sb, "bptc=%x %+v ", p, res)
	rsv, res := rb.BPTCReserved()
	fmt.Fprintf(&sb, "rsv=%v %+v ", rsv, res)
	p34, res := rb.Rate34Payload()
	fmt.Fprintf(&sb, "r34=%x %+v ", p34, res)
	p1, res := rb.Rate1Payload()
	fmt.Fprintf(&sb, "r1=%x %+v ", p1, res)
	frames, res := rb.AMBEFrames()
	fmt.Fprintf(&sb, "ambe=%v %+v", frames, res)
	return sb.String()
}

var dataTypes = []enums.DataType{
	enums.DataTypePIHeader, enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC, enums.DataTypeCSBK,
	enums.DataTypeMBCHeader, enums.DataTypeMBCContinuation, enums.DataTypeDataHeader, enums.DataTypeRate12,
	enums.DataTypeRate34, enums.DataTypeIdle, enums.DataTypeRate1, enums.DataTypeUnifiedSingleBlock,
}

func builtBurst(r *rand.Rand, dt enums.DataType, src layer2.SyncSource) layer2.RawBurst {
	cc := enums.ColorCode(r.Intn(16))
	var rb layer2.RawBurst
	switch dt {
	case enums.DataTypeRate34:
		var p [18]byte
		r.Read(p[:])
		rb, _ = layer2.BuildRate34DataBurst(p, cc, src)
	case enums.DataTypeRate1:
		var p [24]byte
		r.Read(p[:])
		rb, _ = layer2.BuildRate1DataBurst(p, cc, src)
	default:
		var p [12]byte
		r.Read(p[:])
		if r.Intn(4) != 0 {
			_ = pdu.Seal(&p, dt)
		}
		rb, _ = layer2.BuildDataBurst(p, dt, cc, src)
	}
	return rb
}

func voiceBurst(r *rand.Rand, src layer2.SyncSource) layer2.RawBurst {
	var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	for i := range frames {
		randBits(r, frames[i][:])
	}
	var embedded [4]byte
	r.Read(embedded[:])
	emb := pdu.EmbeddedSignalling{ColorCode: enums.ColorCode(r.Intn(16)), PreemptionAndPowerControlIndicator: r.Intn(2) == 1}
	pos := enums.VoiceBurstType(1 + r.Intn(6))
	rb, _ := layer2.BuildVoiceBurst(frames, pos, emb, embedded, src)
	return rb
}

func collectBursts(d digests) {
	r := rand.New(rand.NewSource(1))
	srcs := []layer2.SyncSource{layer2.SyncSourceBS, layer2.SyncSourceMS, layer2.SyncSourceDirectTS1, layer2.SyncSourceDirectTS2}
	var b layer2.Burst
	opts := []*layer2.DecodeOptions{nil, {Kind: layer2.KindData}, {Kind: layer2.KindVoice}, {Kind: layer2.KindReverseChannel}}
	for _, dt := range dataTypes {
		cat := "burst/" + dt.String()
		for n := range 1500 {
			rb := builtBurst(r, dt, srcs[n%len(srcs)])
			corrupt(r, rb[:], 1+n%12)
			err := b.DecodeFromBytesWith(rb, opts[n%2])
			d.add(cat, "%x err=%s %s", rb, errString(err), burstString(&b))
			d.add("raw/"+dt.String(), "%x %s", rb, rawBurstString(rb))
		}
	}
	for n := range 3000 {
		rb := voiceBurst(r, srcs[n%len(srcs)])
		corrupt(r, rb[:], n%10)
		o := opts[n%3]
		if n%7 == 0 {
			o = &layer2.DecodeOptions{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstType(n % 7)}
		}
		err := b.DecodeFromBytesWith(rb, o)
		d.add("burst/voice", "%x err=%s %s", rb, errString(err), burstString(&b))
		d.add("raw/voice", "%x %s", rb, rawBurstString(rb))
	}
	for n := range 5000 {
		var rb layer2.RawBurst
		r.Read(rb[:])
		err := b.DecodeFromBytesWith(rb, opts[n%len(opts)])
		d.add("burst/random", "%x err=%s %s", rb, errString(err), burstString(&b))
		d.add("raw/random", "%x %s", rb, rawBurstString(rb))
	}
	for n := range 2000 {
		rb := builtBurst(r, dataTypes[n%len(dataTypes)], srcs[n%len(srcs)])
		var rc layer2.Burst
		_ = rc.DecodeFromBytes(rb)
		rc.Kind = layer2.KindReverseChannel
		rc.ReverseChannel.RCCommand = enums.RCCommand(r.Intn(16))
		rc.EmbeddedSignalling.ColorCode = enums.ColorCode(r.Intn(16))
		out, err := rc.Encode()
		corrupt(r, out[:], n%6)
		err2 := b.DecodeFromBytesWith(out, nil)
		d.add("burst/rc", "%x %s %s %s", out, errString(err), errString(err2), burstString(&b))
	}
}

func collectBuild(d digests) {
	r := rand.New(rand.NewSource(2))
	var b layer2.Burst
	for _, dt := range dataTypes {
		for range 300 {
			rb := builtBurst(r, dt, layer2.SyncSourceBS)
			_ = b.DecodeFromBytes(rb)
			out, err := layer2.BuildBurst(b.Data, b.SlotType.ColorCode, layer2.SyncSourceMS)
			d.add("build/"+dt.String(), "%x %x %s", rb, out, errString(err))
			var set layer2.RawBurst
			rsv := [3]bit.Bit{bit.Bit(r.Intn(2)), bit.Bit(r.Intn(2)), bit.Bit(r.Intn(2))}
			set = rb
			set.SetBPTCReserved(rsv)
			p, _ := rb.BPTCPayload()
			set2 := rb
			set2.SetBPTCPayload(p)
			p34, _ := rb.Rate34Payload()
			set3 := rb
			set3.SetRate34Payload(p34)
			p1, _ := rb.Rate1Payload()
			set4 := rb
			set4.SetRate1Payload(p1)
			st := rb.SlotType()
			set5 := rb
			set5.SetSlotType(st)
			d.add("set/"+dt.String(), "%x %x %x %x %x", set, set2, set3, set4, set5)
		}
	}
	for range 1000 {
		rb := voiceBurst(r, layer2.SyncSourceMS)
		var s layer2.RawBurst
		s.SetVoiceSlots(rb.VoiceSlots())
		frames, _ := rb.AMBEFrames()
		var s2 layer2.RawBurst
		s2.SetAMBEFrames(frames)
		var s3 layer2.RawBurst
		s3.SetEMB(rb.EMB())
		s3.SetEmbeddedData(rb.EmbeddedData())
		s3.SetStandaloneRCEMB(rb.EMB())
		s3.SetStandaloneRCData(rb.EmbeddedData())
		sync, _ := rb.Sync()
		err := s3.SetSync(sync)
		d.add("set/voice", "%x %x %x %x %s", rb, s, s2, s3, errString(err))
	}
}

func collectPDU(d digests) {
	r := rand.New(rand.NewSource(3))
	for n := range 4000 {
		var b96 [96]bit.Bit
		randBits(r, b96[:])
		if n%2 == 0 {
			p := bit.Pack96(b96)
			_ = pdu.Seal(&p, enums.DataTypeCSBK)
			b96 = bit.Unpack96(p)
		}
		c := pdu.DecodeCSBK(b96)
		enc, err := c.Encode()
		d.add("pdu/csbk", "%v %s %v %s %+v", b96, c.String(), enc, errString(err), c.FEC)
		for _, dt := range []enums.DataType{enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC} {
			x := b96
			if n%2 == 0 {
				p := bit.Pack96(x)
				_ = pdu.Seal(&p, dt)
				x = bit.Unpack96(p)
			}
			lc := pdu.DecodeFullLinkControl(x, dt)
			enc, err := lc.Encode()
			d.add("pdu/flc", "%v %s %v %s %+v", x, lc.String(), enc, errString(err), lc.FEC)
		}
		pi := pdu.DecodePIHeader(b96)
		enc, err = pi.Encode()
		d.add("pdu/pi", "%v %s %v %s", b96, pi.String(), enc, errString(err))
		dh := pdu.DecodeDataHeader(b96)
		enc, err = dh.Encode()
		d.add("pdu/dh", "%v %s %v %s %+v", b96, dh.String(), enc, errString(err), dh.FEC)
		mh := pdu.DecodeMBCHeader(b96)
		enc, err = mh.Encode()
		d.add("pdu/mbch", "%v %s %v %s %+v", b96, mh.String(), enc, errString(err), mh.FEC)
		mc := pdu.DecodeMBCContinuation(b96)
		enc, err = mc.Encode()
		d.add("pdu/mbcc", "%v %s %v %s", b96, mc.String(), enc, errString(err))
		u := pdu.DecodeUnifiedSingleBlockData(b96)
		enc, err = u.Encode()
		d.add("pdu/usbd", "%v %s %v %s %+v", b96, u.String(), enc, errString(err), u.FEC)
		r12 := pdu.DecodeRate12Data(b96)
		enc, err = r12.Encode()
		d.add("pdu/r12", "%v %s %v %s", b96, r12.String(), enc, errString(err))
		var b144 [144]bit.Bit
		randBits(r, b144[:])
		r34 := pdu.DecodeRate34Data(b144)
		e144, err := r34.Encode()
		d.add("pdu/r34", "%s %v %s", r34.String(), e144, errString(err))
		var b192 [192]bit.Bit
		randBits(r, b192[:])
		r1 := pdu.DecodeRate1Data(b192)
		e192, err := r1.Encode()
		d.add("pdu/r1", "%s %v %s", r1.String(), e192, errString(err))
		var b20 [20]bit.Bit
		randBits(r, b20[:])
		st := pdu.DecodeSlotType(b20)
		e20, err := st.Encode()
		d.add("pdu/st", "%v %+v %v %s", b20, st, e20, errString(err))
		var b16 [16]bit.Bit
		randBits(r, b16[:])
		es := pdu.DecodeEmbeddedSignalling(b16)
		e16, err := es.Encode()
		d.add("pdu/emb", "%v %+v %v %s", b16, es, e16, errString(err))
		var b7 [7]bit.Bit
		randBits(r, b7[:])
		ta := pdu.DecodeTACT(b7)
		e7, err := ta.Encode()
		d.add("pdu/tact", "%v %+v %v %s", b7, ta, e7, errString(err))
		var b36 [36]bit.Bit
		randBits(r, b36[:])
		sl := pdu.DecodeShortLC(b36)
		e36, err := sl.Encode()
		d.add("pdu/slc", "%v %s %v %s %+v", b36, sl.String(), e36, errString(err), sl.FEC)
		var b72 [72]bit.Bit
		randBits(r, b72[:])
		udp := pdu.DecodeUDPIPv4CompressedHeader(b72)
		e72, err := udp.Encode()
		d.add("pdu/udp", "%v %s %v %s", b72, udp.String(), e72, errString(err))
		var b11 [11]bit.Bit
		randBits(r, b11[:])
		rc := pdu.DecodeReverseChannel(b11)
		e11, err := rc.Encode()
		d.add("pdu/rc", "%v %+v %v %s", b11, rc, e11, errString(err))
		var b8 [8]bit.Bit
		randBits(r, b8[:])
		so := layer3.DecodeServiceOptions(b8)
		e8, err := so.Encode()
		d.add("pdu/so", "%v %s %v %s", b8, so.String(), e8, errString(err))
		var b58 [58]bit.Bit
		randBits(r, b58[:])
		cp := layer3.DecodeCdefParms(b58)
		e58, err := cp.Encode()
		d.add("pdu/cdef", "%v %+v %v %s", b58, cp, e58, errString(err))
		cw := uint32(r.Intn(1 << 20))
		sti, res := pdu.DecodeSlotTypeCodeword(cw)
		d.add("pdu/stcw", "%x %+v %+v", cw, sti, res)
		ecw := uint16(r.Intn(1 << 16))
		ei, res := pdu.DecodeEMBCodeword(ecw)
		d.add("pdu/embcw", "%x %+v %+v", ecw, ei, res)
		var cw12 [12]byte
		r.Read(cw12[:])
		for _, dt := range []enums.DataType{enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC} {
			x := cw12
			if n%2 == 0 {
				_ = pdu.Seal(&x, dt)
				corrupt(r, x[:], 5)
			}
			lc, res, err := pdu.DecodeFullLCCodeword(x, dt)
			d.add("pdu/flccw", "%x %x %+v %s %v", x, lc, res, errString(err), pdu.FullLCParityOK(x, dt))
		}
		d.add("pdu/hash", "%d", pdu.AddressHash(uint32(r.Intn(1<<24))))
	}
}

func collectEmbedded(d digests) {
	r := rand.New(rand.NewSource(4))
	var asm layer2.EmbeddedLCAssembler
	for n := range 4000 {
		var lc [9]byte
		r.Read(lc[:])
		f := layer2.EncodeEmbeddedLC(lc)
		d.add("emb/encode", "%x %x", lc, f)
		for i := range f {
			corrupt(r, f[i][:], n%5)
		}
		if n%10 == 0 {
			for i := range f {
				r.Read(f[i][:])
			}
		}
		got, res := layer2.DecodeEmbeddedLC(f, false)
		gotS, resS := layer2.DecodeEmbeddedLC(f, true)
		d.add("emb/decode", "%x %x %+v %x %+v", f, got, res, gotS, resS)
		flc := layer2.DecodeEmbeddedFullLinkControl(f, n%2 == 0)
		d.add("emb/flc", "%s %+v", flc.String(), flc.FEC)
		lcss := []enums.LCSS{enums.LCSSFirstFragment, enums.LCSSContinuationFragment, enums.LCSSContinuationFragment, enums.LCSSLastFragment}
		for i := range f {
			l := lcss[i]
			if r.Intn(20) == 0 {
				l = enums.LCSS(r.Intn(4))
			}
			lc, res, done, err := asm.Add(f[i], l)
			d.add("emb/asm", "%x %+v %v %s", lc, res, done, errString(err))
		}
		var e32 [32]bit.Bit
		randBits(r, e32[:])
		rc := layer2.DecodeEmbeddedRC(e32)
		d.add("emb/rc", "%v %+v %v", e32, rc, layer2.IsNullEmbeddedLC(e32))
		rc.RCCommand = enums.RCCommand(r.Intn(16))
		enc, err := layer2.EncodeEmbeddedRC(&rc)
		d.add("emb/rcenc", "%v %s", enc, errString(err))
	}
}

func collectCACH(d digests) {
	r := rand.New(rand.NewSource(5))
	var asm layer2.ShortLCAssembler
	for n := range 4000 {
		var b [layer2.CACHBits]bit.Bit
		randBits(r, b[:])
		c := layer2.DecodeCACH(b)
		cf := layer2.DecodeCACHField(b)
		d.add("cach/decode", "%v %+v %+v %v %v", b, c, cf, layer2.CACHDeinterleave(b), layer2.CACHInterleave(b))
		slc, done, err := asm.Add(cf)
		d.add("cach/asm", "%s %v %s", slc.String(), done, errString(err))
		var s36 [36]bit.Bit
		randBits(r, s36[:])
		sl := pdu.DecodeShortLC(s36)
		ta := pdu.TACT{AccessType: r.Intn(2) == 1, TDMAChannel: r.Intn(2) == 1}
		frags, err := layer2.EncodeShortLCToCACH(&sl, ta)
		d.add("cach/encode", "%v %s", frags, errString(err))
		for i := range frags {
			if n%3 == 0 {
				frags[i][r.Intn(layer2.CACHBits)] ^= 1
			}
			s, done, err := asm.Add(layer2.DecodeCACHField(frags[i]))
			d.add("cach/asm2", "%s %v %s", s.String(), done, errString(err))
		}
	}
}

func collectVocoder(d digests) {
	r := rand.New(rand.NewSource(6))
	for n := range 20000 {
		var slot [72]bit.Bit
		randBits(r, slot[:])
		f, res := vocoder.DecodeAMBE72(slot)
		d.add("ambe/decode", "%v %v %+v", slot, f, res)
		var fr vocoder.AMBEFrame
		randBits(r, fr[:])
		enc := vocoder.EncodeAMBE72(fr)
		corrupt2 := enc
		for range n % 6 {
			corrupt2[r.Intn(72)] ^= 1
		}
		g, res2 := vocoder.DecodeAMBE72(corrupt2)
		d.add("ambe/encode", "%v %v %v %+v", fr, enc, g, res2)
	}
}

func collectFramer(d digests) {
	r := rand.New(rand.NewSource(7))
	stream := make([]bit.Bit, 0, 264*40)
	for n := range 40 {
		rb := builtBurst(r, dataTypes[n%len(dataTypes)], layer2.SyncSourceBS)
		if n%3 == 0 {
			rb = voiceBurst(r, layer2.SyncSourceBS)
		}
		corrupt(r, rb[:], n%4)
		stream = bit.AppendUnpack(stream, rb[:])
		var junk [37]bit.Bit
		randBits(r, junk[:n%37])
		stream = append(stream, junk[:n%37]...)
	}
	for start := 0; start < len(stream); start += 97 {
		m, ok := framer.Search(stream, start, 6)
		d.add("framer/search", "%d %+v %v", start, m, ok)
	}
	f := framer.New(framer.Config{})
	var out []framer.Burst
	for i := 0; i < len(stream); i += 101 {
		out = f.AppendBursts(out[:0], stream[i:min(i+101, len(stream))]...)
		for _, b := range out {
			d.add("framer/write", "%+v", b)
		}
	}
}

func TestDigests(t *testing.T) {
	t.Parallel()

	d := digests{}
	collectBursts(d)
	collectBuild(d)
	collectPDU(d)
	collectEmbedded(d)
	collectCACH(d)
	collectVocoder(d)
	collectFramer(d)

	got := map[string]string{}
	for k, h := range d {
		got[k] = hex.EncodeToString(h.Sum(nil))
	}
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	if *update {
		var sb strings.Builder
		for _, k := range keys {
			fmt.Fprintf(&sb, "%s %s\n", k, got[k])
		}
		if err := os.WriteFile(goldenFile, []byte(sb.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}

	f, err := os.Open(goldenFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	want := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		i := strings.LastIndexByte(line, ' ')
		want[line[:i]] = line[i+1:]
	}
	for _, k := range keys {
		if want[k] != got[k] {
			t.Errorf("%s: digest %s, want %s", k, got[k], want[k])
		}
	}
	for k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("%s: missing", k)
		}
	}
}
