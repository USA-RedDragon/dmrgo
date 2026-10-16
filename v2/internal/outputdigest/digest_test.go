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

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
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

func randBits(r *rand.Rand, dst []unpacked.Bit) {
	for i := range dst {
		dst[i] = unpacked.Bit(r.Intn(2))
	}
}

// pack returns b packed, the first bit in the most significant bit of byte 0.
func pack(b []unpacked.Bit) []byte {
	p := make([]byte, (len(b)+7)/8)
	unpacked.PackAt(p, 0, b)
	return p
}

// unpack returns the first n bits of p, one unpacked.Bit per bit, the form that
// the digests print.
func unpack(p []byte, n int) []unpacked.Bit {
	b := make([]unpacked.Bit, n)
	unpacked.UnpackAt(b, p, 0)
	return b
}

// reserved returns the BPTC reserved bits res, R(i) in bit i, as the digests
// print them: R(i) at index i.
func reserved(res uint8) [3]unpacked.Bit {
	return [3]unpacked.Bit{unpacked.Bit(res & 1), unpacked.Bit(res >> 1 & 1), unpacked.Bit(res >> 2 & 1)}
}

// slotBits returns the three voice slots s as the digests print them: 72
// unpacked.Bit values each.
func slotBits(s [layer2.VoiceSlotsPerBurst][9]byte) [layer2.VoiceSlotsPerBurst][]unpacked.Bit {
	var out [layer2.VoiceSlotsPerBurst][]unpacked.Bit
	for i := range s {
		out[i] = unpack(s[i][:], 72)
	}
	return out
}

// frameBits returns the three AMBE+2 frames f as the digests print them: 49
// unpacked.Bit values each.
func frameBits(f [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame) [layer2.VoiceSlotsPerBurst][]unpacked.Bit {
	var out [layer2.VoiceSlotsPerBurst][]unpacked.Bit
	for i := range f {
		out[i] = unpack(f[i][:], 49)
	}
	return out
}

// cachString returns c in the form that %+v printed when CACH.Payload held
// one unpacked.Bit per bit.
func cachString(c layer2.CACH) string {
	return fmt.Sprintf("{TACT:%+v Payload:%v}", c.TACT, unpack(c.Payload[:], layer2.CACHPayloadBits))
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
	enc, err := encodeData(d)
	return fmt.Sprintf("%T %s enc=%x err=%s", d, d.String(), enc, errString(err))
}

// encodeData returns the packed bits of the Encode method of d as a slice,
// or nil and the error of Encode.
func encodeData(d layer2.Data) ([]byte, error) {
	var enc []byte
	var err error
	switch e := d.(type) {
	case interface{ Encode() ([12]byte, error) }:
		var a [12]byte
		a, err = e.Encode()
		enc = a[:]
	case interface{ Encode() ([18]byte, error) }:
		var a [18]byte
		a, err = e.Encode()
		enc = a[:]
	case interface{ Encode() ([24]byte, error) }:
		var a [24]byte
		a, err = e.Encode()
		enc = a[:]
	default:
		return nil, fmt.Errorf("%T has no Encode method", d)
	}
	if err != nil {
		return nil, err
	}
	return enc, nil
}

func burstString(b *layer2.Burst) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "k=%v sp=%v ns=%v sd=%d vb=%v vs=%v st=%+v rsv=%v emb=%+v ed=%v rc=%+v fec=%+v hes=%v hrc=%v ",
		b.Kind, b.SyncPattern, b.NearestSync, b.SyncDistance, b.VoiceBurst, slotBits(b.VoiceSlots), b.SlotType, reserved(b.BPTCReserved),
		b.EmbeddedSignalling, unpack(b.EmbeddedSignallingData[:], 32), b.ReverseChannel, b.FEC, b.HasEmbeddedSignalling(), b.HasReverseChannel())
	sb.WriteString(dataString(b.Data))
	out, err := b.Encode()
	fmt.Fprintf(&sb, " out=%x err=%s", out, errString(err))
	frames, res := b.AMBEFrames()
	fmt.Fprintf(&sb, " ambe=%v %+v", frameBits(frames), res)
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
	fmt.Fprintf(&sb, "rsv=%v %+v ", reserved(rsv), res)
	p34, res := rb.Rate34Payload()
	fmt.Fprintf(&sb, "r34=%x %+v ", p34, res)
	p1, res := rb.Rate1Payload()
	fmt.Fprintf(&sb, "r1=%x %+v ", p1, res)
	frames, res := rb.AMBEFrames()
	fmt.Fprintf(&sb, "ambe=%v %+v", frameBits(frames), res)
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
		var b [49]unpacked.Bit
		randBits(r, b[:])
		frames[i] = vocoder.AMBEFrame(pack(b[:]))
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
			rsv := uint8(r.Intn(2) | r.Intn(2)<<1 | r.Intn(2)<<2)
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
		var b96 [96]unpacked.Bit
		randBits(r, b96[:])
		p96 := [12]byte(pack(b96[:]))
		if n%2 == 0 {
			_ = pdu.Seal(&p96, enums.DataTypeCSBK)
		}
		c := pdu.DecodeCSBK(p96)
		enc, err := c.Encode()
		d.add("pdu/csbk", "%v %s %v %s %+v", unpack(p96[:], 96), c.String(), unpack(enc[:], 96), errString(err), c.FEC)
		for _, dt := range []enums.DataType{enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC} {
			x := p96
			if n%2 == 0 {
				_ = pdu.Seal(&x, dt)
			}
			lc := pdu.DecodeFullLinkControl(x, dt)
			enc, err := lc.Encode()
			d.add("pdu/flc", "%v %s %v %s %+v", unpack(x[:], 96), lc.String(), unpack(enc[:], 96), errString(err), lc.FEC)
		}
		pi := pdu.DecodePIHeader(p96)
		enc, err = pi.Encode()
		d.add("pdu/pi", "%v %s %v %s", unpack(p96[:], 96), pi.String(), unpack(enc[:], 96), errString(err))
		dh := pdu.DecodeDataHeader(p96)
		enc, err = dh.Encode()
		d.add("pdu/dh", "%v %s %v %s %+v", unpack(p96[:], 96), dh.String(), unpack(enc[:], 96), errString(err), dh.FEC)
		mh := pdu.DecodeMBCHeader(p96)
		enc, err = mh.Encode()
		d.add("pdu/mbch", "%v %s %v %s %+v", unpack(p96[:], 96), mh.String(), unpack(enc[:], 96), errString(err), mh.FEC)
		mc := pdu.DecodeMBCContinuation(p96)
		enc, err = mc.Encode()
		d.add("pdu/mbcc", "%v %s %v %s", unpack(p96[:], 96), mc.String(), unpack(enc[:], 96), errString(err))
		u := pdu.DecodeUnifiedSingleBlockData(p96)
		enc, err = u.Encode()
		d.add("pdu/usbd", "%v %s %v %s %+v", unpack(p96[:], 96), u.String(), unpack(enc[:], 96), errString(err), u.FEC)
		r12 := pdu.DecodeRate12Data(p96)
		enc, err = r12.Encode()
		d.add("pdu/r12", "%v %s %v %s", unpack(p96[:], 96), r12.String(), unpack(enc[:], 96), errString(err))
		var b144 [144]unpacked.Bit
		randBits(r, b144[:])
		r34 := pdu.DecodeRate34Data([18]byte(pack(b144[:])))
		e144, err := r34.Encode()
		d.add("pdu/r34", "%s %v %s", r34.String(), unpack(e144[:], 144), errString(err))
		var b192 [192]unpacked.Bit
		randBits(r, b192[:])
		r1 := pdu.DecodeRate1Data([24]byte(pack(b192[:])))
		e192, err := r1.Encode()
		d.add("pdu/r1", "%s %v %s", r1.String(), unpack(e192[:], 192), errString(err))
		var b20 [20]unpacked.Bit
		randBits(r, b20[:])
		st := pdu.DecodeSlotType([3]byte(pack(b20[:])))
		e20, err := st.Encode()
		d.add("pdu/st", "%v %+v %v %s", b20, st, unpack(e20[:], 20), errString(err))
		var b16 [16]unpacked.Bit
		randBits(r, b16[:])
		es := pdu.DecodeEmbeddedSignalling([2]byte(pack(b16[:])))
		e16, err := es.Encode()
		d.add("pdu/emb", "%v %+v %v %s", b16, es, unpack(e16[:], 16), errString(err))
		var b7 [7]unpacked.Bit
		randBits(r, b7[:])
		ta := pdu.DecodeTACT([1]byte(pack(b7[:])))
		e7, err := ta.Encode()
		d.add("pdu/tact", "%v %+v %v %s", b7, ta, unpack(e7[:], 7), errString(err))
		var b36 [36]unpacked.Bit
		randBits(r, b36[:])
		sl := pdu.DecodeShortLC([5]byte(pack(b36[:])))
		e36, err := sl.Encode()
		d.add("pdu/slc", "%v %s %v %s %+v", b36, sl.String(), unpack(e36[:], 36), errString(err), sl.FEC)
		var b72 [72]unpacked.Bit
		randBits(r, b72[:])
		udp := pdu.DecodeUDPIPv4CompressedHeader([9]byte(pack(b72[:])))
		e72, err := udp.Encode()
		d.add("pdu/udp", "%v %s %v %s", b72, udp.String(), unpack(e72[:], 72), errString(err))
		var b11 [11]unpacked.Bit
		randBits(r, b11[:])
		rc := pdu.DecodeReverseChannel([2]byte(pack(b11[:])))
		e11, err := rc.Encode()
		d.add("pdu/rc", "%v %+v %v %s", b11, rc, unpack(e11[:], 11), errString(err))
		var b8 [8]unpacked.Bit
		randBits(r, b8[:])
		so := layer3.DecodeServiceOptions([1]byte(pack(b8[:])))
		e8, err := so.Encode()
		d.add("pdu/so", "%v %s %v %s", b8, so.String(), unpack(e8[:], 8), errString(err))
		var b58 [58]unpacked.Bit
		randBits(r, b58[:])
		cp := layer3.DecodeCdefParms([8]byte(pack(b58[:])))
		e58, err := cp.Encode()
		d.add("pdu/cdef", "%v %+v %v %s", b58, cp, unpack(e58[:], 58), errString(err))
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
		var e32 [32]unpacked.Bit
		randBits(r, e32[:])
		p32 := [4]byte(pack(e32[:]))
		rc := layer2.DecodeEmbeddedRC(p32)
		d.add("emb/rc", "%v %+v %v", e32, rc, layer2.IsNullEmbeddedLC(p32))
		rc.RCCommand = enums.RCCommand(r.Intn(16))
		enc, err := layer2.EncodeEmbeddedRC(&rc)
		d.add("emb/rcenc", "%v %s", unpack(enc[:], 32), errString(err))
	}
}

func collectCACH(d digests) {
	r := rand.New(rand.NewSource(5))
	var asm layer2.ShortLCAssembler
	for n := range 4000 {
		var b [layer2.CACHBits]unpacked.Bit
		randBits(r, b[:])
		p := [3]byte(pack(b[:]))
		c := layer2.DecodeCACH(p)
		cf := layer2.DecodeCACHField(p)
		de, in := layer2.CACHDeinterleave(p), layer2.CACHInterleave(p)
		d.add("cach/decode", "%v %s %s %v %v", b, cachString(c), cachString(cf), unpack(de[:], 24), unpack(in[:], 24))
		slc, done, err := asm.Add(cf)
		d.add("cach/asm", "%s %v %s", slc.String(), done, errString(err))
		var s36 [36]unpacked.Bit
		randBits(r, s36[:])
		sl := pdu.DecodeShortLC([5]byte(pack(s36[:])))
		ta := pdu.TACT{AccessType: r.Intn(2) == 1, TDMAChannel: r.Intn(2) == 1}
		frags, err := layer2.EncodeShortLCToCACH(&sl, ta)
		var fragBits [4][]unpacked.Bit
		for i := range frags {
			fragBits[i] = unpack(frags[i][:], layer2.CACHBits)
		}
		d.add("cach/encode", "%v %s", fragBits, errString(err))
		for i := range frags {
			if n%3 == 0 {
				j := r.Intn(layer2.CACHBits)
				frags[i][j/8] ^= 0x80 >> (j % 8)
			}
			s, done, err := asm.Add(layer2.DecodeCACHField(frags[i]))
			d.add("cach/asm2", "%s %v %s", s.String(), done, errString(err))
		}
	}
}

func collectVocoder(d digests) {
	r := rand.New(rand.NewSource(6))
	for n := range 20000 {
		var slot [72]unpacked.Bit
		randBits(r, slot[:])
		f, res := vocoder.DecodeAMBE72([9]byte(pack(slot[:])))
		d.add("ambe/decode", "%v %v %+v", slot, unpack(f[:], 49), res)
		var fr [49]unpacked.Bit
		randBits(r, fr[:])
		enc := vocoder.EncodeAMBE72(vocoder.AMBEFrame(pack(fr[:])))
		corrupt2 := enc
		for range n % 6 {
			j := r.Intn(72)
			corrupt2[j/8] ^= 0x80 >> (j % 8)
		}
		g, res2 := vocoder.DecodeAMBE72(corrupt2)
		d.add("ambe/encode", "%v %v %v %+v", fr, unpack(enc[:], 72), unpack(g[:], 49), res2)
	}
}

func collectFramer(d digests) {
	r := rand.New(rand.NewSource(7))
	stream := make([]unpacked.Bit, 0, 264*40)
	for n := range 40 {
		rb := builtBurst(r, dataTypes[n%len(dataTypes)], layer2.SyncSourceBS)
		if n%3 == 0 {
			rb = voiceBurst(r, layer2.SyncSourceBS)
		}
		corrupt(r, rb[:], n%4)
		stream = append(stream, unpack(rb[:], layer2.BurstBits)...)
		var junk [37]unpacked.Bit
		randBits(r, junk[:n%37])
		stream = append(stream, junk[:n%37]...)
	}
	p := pack(stream)
	for start := 0; start < len(stream); start += 97 {
		m, ok := framer.Search(p, start, 6)
		d.add("framer/search", "%d %+v %v", start, m, ok)
	}
	f := framer.New(framer.Config{})
	var out []framer.Burst
	for i := 0; i < len(p); i += 13 {
		out = f.AppendBursts(out[:0], p[i:min(i+13, len(p))])
		for _, b := range out {
			d.add("framer/write", "{Data:%v Offset:%v Sync:%v Distance:%v HasCACH:%v CACH:%s Channel:%v}",
				b.Data, b.Offset, b.Sync, b.Distance, b.HasCACH, cachString(b.CACH), b.Channel)
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
