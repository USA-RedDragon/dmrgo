//go:build !tinygo

package zeroalloc_test

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/ambe"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/golay"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/hamming"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/quadraticresidue"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/trellis"
)

// Bit offsets of the fields of a 264-bit burst (ETSI TS 102 361-1 clause
// 4.2.2).
const (
	infoHalfBits   = 98
	slotTypeStart1 = 98
	centerStart    = 108
	embeddedStart  = 116
	embeddedEnd    = 148
	centerEnd      = 156
	slotTypeStart2 = 156
	slotTypeHalf   = 10
	infoStart2     = 166
)

// captureRawBursts returns the 33-byte bursts of a capture in
// layer2/testdata.
func captureRawBursts(t testing.TB, name string) [][33]byte {
	t.Helper()
	data, err := os.ReadFile("../../layer2/testdata/" + name + ".bin")
	if err != nil || len(data)%33 != 0 {
		t.Fatalf("read %s: %v", name, err)
	}
	out := make([][33]byte, 0, len(data)/33)
	for i := 0; i < len(data); i += 33 {
		out = append(out, [33]byte(data[i:i+33]))
	}
	return out
}

// burstFields holds the FEC-protected fields of one captured burst.
type burstFields struct {
	info     [196]bit.Bit
	slotType [20]bit.Bit
	emb      [16]bit.Bit
	embedded [32]bit.Bit
}

func splitBurst(b [33]byte) burstFields {
	bits := bit.Unpack264(b)
	var f burstFields
	copy(f.info[:infoHalfBits], bits[:infoHalfBits])
	copy(f.info[infoHalfBits:], bits[infoStart2:])
	copy(f.slotType[:slotTypeHalf], bits[slotTypeStart1:centerStart])
	copy(f.slotType[slotTypeHalf:], bits[slotTypeStart2:infoStart2])
	copy(f.emb[:8], bits[centerStart:embeddedStart])
	copy(f.emb[8:], bits[embeddedEnd:centerEnd])
	copy(f.embedded[:], bits[embeddedStart:embeddedEnd])
	return f
}

// dataType returns the data type of a slot type codeword that
// golay.Decode2087 corrected.
func dataType(slotType uint32) enums.DataType {
	return enums.DataType(slotType >> 12 & 0xF)
}

// embeddedLC returns the 72 LC bits and the 5 checksum bits of the 77
// information bits of an embedded LC (ETSI TS 102 361-1 Figure B.3).
func embeddedLC(info [77]bit.Bit) ([9]byte, uint8) {
	var lc [72]bit.Bit
	var cs [5]bit.Bit
	n := 0
	for row := range 7 {
		r := info[row*11 : row*11+11]
		if row < 2 {
			n += copy(lc[n:], r)
			continue
		}
		n += copy(lc[n:], r[:10])
		cs[row-2] = r[10]
	}
	var out [9]byte
	bit.AppendPack(out[:0], lc[:])
	return out, uint8(bit.Uint(cs[:]))
}

func embeddedLCValid(info [77]bit.Bit) bool {
	lc, cs := embeddedLC(info)
	return crc.CheckChecksum5(lc, cs)
}

func flip[T ~[]bit.Bit](bits T, pos ...int) {
	for _, p := range pos {
		bits[p] ^= 1
	}
}

func patternBits[T ~[]bit.Bit](bits T, seed int) {
	for i := range bits {
		bits[i] = bit.Bit((i*seed + i/3) & 1)
	}
}

// fecEntries returns the entries of package fec and the internal FEC and
// CRC packages, with fields of the captures in layer2/testdata where the
// capture has them.
func fecEntries(t testing.TB) []entry {
	t.Helper()
	voice := captureRawBursts(t, "voice")
	sms := captureRawBursts(t, "d-sms")

	header := splitBurst(voice[0])
	lcBits, res := bptc.Decode196(header.info)
	if res.HasErrors() {
		t.Fatalf("voice LC header BPTC: %+v", res)
	}
	headerSTCW := uint32(bit.Uint(header.slotType[:]))
	headerST, res := golay.Decode2087(headerSTCW)
	if res.HasErrors() || dataType(headerST) != enums.DataTypeVoiceLCHeader {
		t.Fatalf("voice LC header slot type: %v %+v", dataType(headerST), res)
	}
	lc := bit.Pack96(lcBits)
	for i := 9; i < 12; i++ {
		lc[i] ^= 0x96
	}
	if !reedsolomon.Check(lc) {
		t.Fatalf("voice LC header % X is not a Reed-Solomon codeword", lc)
	}
	lcBad := lc
	lcBad[4] ^= 0x81
	infoBad := header.info
	flip(infoBad[:], 7, 120)
	stBad := headerSTCW ^ 1<<17 ^ 1<<4

	voiceB := splitBurst(voice[2])
	embCW := uint16(bit.Uint(voiceB.emb[:]))
	if _, res := quadraticresidue.Decode(embCW); res.HasErrors() {
		t.Fatalf("voice B EMB: %+v", res)
	}
	embBad := embCW ^ 1<<12
	var fragments [4][32]bit.Bit
	for i := range fragments {
		fragments[i] = splitBurst(voice[2+i]).embedded
	}
	embInfo, res := bptc.DecodeEmbedded(fragments)
	if res.HasErrors() || !embeddedLCValid(embInfo) {
		t.Fatalf("embedded LC: %+v", res)
	}
	embLC, embCS := embeddedLC(embInfo)
	fragmentsBad := fragments
	flip(fragmentsBad[1][:], 9)

	csbk := splitBurst(sms[0])
	csbkBits, res := bptc.Decode196(csbk.info)
	if res.HasErrors() {
		t.Fatalf("CSBK BPTC: %+v", res)
	}
	csbkBytes := bit.Pack96(csbkBits)
	csbkCRC := binary.BigEndian.Uint16(csbkBytes[10:]) ^ 0xA5A5
	if !crc.CheckCCITT(csbkBytes[:10], csbkCRC) {
		t.Fatalf("CSBK % X: bad CRC-CCITT", csbkBytes)
	}

	var message []byte
	var rate12 [196]bit.Bit
	for _, b := range sms {
		f := splitBurst(b)
		st, _ := golay.Decode2087(uint32(bit.Uint(f.slotType[:])))
		if dataType(st) != enums.DataTypeRate12 {
			continue
		}
		rate12 = f.info
		block, _ := bptc.Decode196(f.info)
		p := bit.Pack96(block)
		message = append(message, p[:]...)
	}
	msgCRC := binary.LittleEndian.Uint32(message[len(message)-4:])
	message = message[:len(message)-4]
	if !crc.CheckCRC32(message, msgCRC) {
		t.Fatalf("d-sms message % X: bad CRC-32", message)
	}

	ambeSlot := [9]byte(voice[2][:9])

	var rate34Bits [144]bit.Bit
	patternBits(rate34Bits[:], 5)
	rate34 := [18]byte(bit.Pack(rate34Bits[:]))
	trellisCW := trellis.Encode(rate34)
	trellisBad := trellisCW
	trellisBad[1] ^= 0x10
	trellisBad[12] ^= 0x08

	var cachInfo [36]bit.Bit
	patternBits(cachInfo[:], 3)
	cach := bptc.EncodeCACH(cachInfo)
	cachBad := cach
	flip(cachBad[2][:], 5)

	var rcInfo [11]bit.Bit
	patternBits(rcInfo[:], 7)
	rc := bptc.EncodeSingleBurst(rcInfo, true)
	rcBad := rc
	flip(rcBad[:], 4, 20)

	tact := uint8(0b1010)
	tactCW := hamming.Encode743(tact)
	tactBad := tactCW ^ 1

	ambeA := golay.Encode24128(0xA5C)
	ambeB := golay.Encode23127(0x3B1)

	var reserved [3]bit.Bit
	var rcCRCBits [4]bit.Bit
	patternBits(rcCRCBits[:], 3)
	rcInfoBytes := bit.Pack(rcInfo[:])
	lcBytes := bit.Pack96(lcBits)
	rate12Bytes := bit.Pack(rate12[:])

	var (
		bits96     [96]bit.Bit
		bits196    [196]bit.Bit
		rate34Out  [18]byte
		ambeFrame  [7]byte
		trellisOut [25]byte
		bits77     [77]bit.Bit
		bits36     [36]bit.Bit
		bits11     [11]bit.Bit
		bits20     [20]bit.Bit
		bits16     [16]bit.Bit
		bits7      [7]bit.Bit
		frags      [4][32]bit.Bit
		cachOut    [4][17]bit.Bit
		bits32     [32]bit.Bit
		rsData     [9]byte
		rsCW       [12]byte
		u16        uint16
		u32        uint32
		u8         uint8
		ok         bool
		r          fec.Result
		err        error
		ber        float64
		n          int
	)
	var stats fec.BurstFECStats
	stats.SlotType = fec.Result{BitsChecked: 20, ErrorsCorrected: 1}
	stats.Payload = fec.Result{BitsChecked: 196, ErrorsCorrected: 2}
	stats.PDU = fec.Result{Uncorrectable: true}
	var calc fec.BERCalculator
	bad := fec.Result{BitsChecked: 96, Uncorrectable: true}

	out := []entry{
		{"fec.Result.Combine", "", 0, func() { r = stats.Payload.Combine(stats.SlotType) }},
		{"fec.Result.Err", "clean", 0, func() { err = stats.SlotType.Err() }},
		{"fec.Result.Err", "uncorrectable", 0, func() { err = bad.Err() }},
		{"fec.Result.HasErrors", "", 0, func() { ok = stats.Payload.HasErrors() }},
		{"fec.BurstFECStats.Aggregate", "", 0, func() { r = stats.Aggregate() }},
		{"fec.BERCalculator.Add", "", 0, func() { calc.Add(stats.Payload) }},
		{"fec.BERCalculator.AddBurst", "", 0, func() { calc.AddBurst(stats) }},
		{"fec.BERCalculator.BER", "", 0, func() { ber = calc.BER() }},
		{"fec.BERCalculator.TotalErrors", "", 0, func() { n = calc.TotalErrors() }},
		{"fec.BERCalculator.TotalBitsChecked", "", 0, func() { n = calc.TotalBitsChecked() }},
		{"fec.BERCalculator.Uncorrectable", "", 0, func() { n = calc.Uncorrectable() }},
		{"fec.BERCalculator.Reset", "", 0, func() { calc.Reset() }},

		{"internal/fec/bptc.Decode196", "voice LC header", 0, func() { bits96, r = bptc.Decode196(header.info) }},
		{"internal/fec/bptc.Decode196", "voice LC header, 2 errors", 0, func() { bits96, r = bptc.Decode196(infoBad) }},
		{"internal/fec/bptc.Decode196", "rate 1/2", 0, func() { bits96, r = bptc.Decode196(rate12) }},
		{"internal/fec/bptc.Decode196WithReserved", "CSBK", 0, func() { bits96, reserved, r = bptc.Decode196WithReserved(csbk.info) }},
		{"internal/fec/bptc.Encode196", "voice LC header", 0, func() { bits196 = bptc.Encode196(lcBits) }},
		{"internal/fec/bptc.Encode196WithReserved", "CSBK", 0, func() { bits196 = bptc.Encode196WithReserved(csbkBits, reserved) }},
		{"internal/fec/bptc.DecodeEmbedded", "voice B to E", 0, func() { bits77, r = bptc.DecodeEmbedded(fragments) }},
		{"internal/fec/bptc.DecodeEmbedded", "voice B to E, 1 error", 0, func() { bits77, r = bptc.DecodeEmbedded(fragmentsBad) }},
		{"internal/fec/bptc.DecodeEmbeddedWithCheck", "voice B to E, 1 error", 0, func() { bits77, r = bptc.DecodeEmbeddedWithCheck(fragmentsBad, embeddedLCValid) }},
		{"internal/fec/bptc.DecodeEmbeddedStrict", "voice B to E, 1 error", 0, func() { bits77, r = bptc.DecodeEmbeddedStrict(fragmentsBad) }},
		{"internal/fec/bptc.EncodeEmbedded", "voice B to E", 0, func() { frags = bptc.EncodeEmbedded(embInfo) }},
		{"internal/fec/bptc.DecodeCACH", "1 error", 0, func() { bits36, r = bptc.DecodeCACH(cachBad) }},
		{"internal/fec/bptc.EncodeCACH", "", 0, func() { cachOut = bptc.EncodeCACH(cachInfo) }},
		{"internal/fec/bptc.DecodeSingleBurst", "reverse channel, 2 errors", 0, func() { bits11, r = bptc.DecodeSingleBurst(rcBad, true) }},
		{"internal/fec/bptc.EncodeSingleBurst", "reverse channel", 0, func() { bits32 = bptc.EncodeSingleBurst(rcInfo, true) }},

		{"internal/fec/golay.Decode2087", "voice LC header slot type", 0, func() { u32, r = golay.Decode2087(headerSTCW) }},
		{"internal/fec/golay.Decode2087", "voice LC header slot type, 2 errors", 0, func() { u32, r = golay.Decode2087(stBad) }},
		{"internal/fec/golay.Check2087", "voice LC header slot type", 0, func() { ok = golay.Check2087(headerSTCW) }},
		{"internal/fec/golay.Encode2087", "voice LC header slot type", 0, func() { u32 = golay.Encode2087(uint8(headerST >> 12)) }},
		{"internal/fec/golay.Decode24128", "1 error", 0, func() { u16, r = golay.Decode24128(ambeA ^ 0x10) }},
		{"internal/fec/golay.Encode24128", "", 0, func() { u32 = golay.Encode24128(0xA5C) }},
		{"internal/fec/golay.Decode23127", "1 error", 0, func() { u16, r = golay.Decode23127(ambeB ^ 0x400) }},
		{"internal/fec/golay.Encode23127", "", 0, func() { u32 = golay.Encode23127(0x3B1) }},

		{"internal/fec/hamming.Decode743", "1 error", 0, func() { u8, r = hamming.Decode743(tactBad) }},
		{"internal/fec/hamming.Check743", "", 0, func() { ok = hamming.Check743(tactCW) }},
		{"internal/fec/hamming.Encode743", "", 0, func() { u8 = hamming.Encode743(tact) }},

		{"internal/fec/quadraticresidue.Decode", "voice B EMB", 0, func() { u16, r = quadraticresidue.Decode(embCW) }},
		{"internal/fec/quadraticresidue.Decode", "voice B EMB, 1 error", 0, func() { u16, r = quadraticresidue.Decode(embBad) }},
		{"internal/fec/quadraticresidue.Check", "voice B EMB", 0, func() { ok = quadraticresidue.Check(embCW) }},
		{"internal/fec/quadraticresidue.Encode", "voice B EMB", 0, func() { u16 = quadraticresidue.Encode(uint8(embCW >> 9)) }},

		{"internal/fec/reedsolomon.Decode", "voice LC header", 0, func() { rsData, r = reedsolomon.Decode(lc) }},
		{"internal/fec/reedsolomon.Decode", "voice LC header, 1 symbol error", 0, func() { rsData, r = reedsolomon.Decode(lcBad) }},
		{"internal/fec/reedsolomon.Check", "voice LC header", 0, func() { ok = reedsolomon.Check(lc) }},
		{"internal/fec/reedsolomon.Encode", "voice LC header", 0, func() { rsCW = reedsolomon.Encode([9]byte(lc[:9])) }},

		{"internal/fec/trellis.Decode", "", 0, func() { rate34Out, r = trellis.Decode(trellisCW) }},
		{"internal/fec/trellis.Decode", "2 errors", 0, func() { rate34Out, r = trellis.Decode(trellisBad) }},
		{"internal/fec/trellis.Encode", "", 0, func() { trellisOut = trellis.Encode(rate34) }},

		{"internal/fec/ambe.Decode", "voice B slot 0", 0, func() { ambeFrame, r = ambe.Decode(ambeSlot) }},
		{"internal/fec/ambe.Encode", "voice B slot 0", 0, func() { ambeSlot = ambe.Encode(ambeFrame) }},

		{"internal/crc.CCITT", "CSBK", 0, func() { u16 = crc.CCITT(csbkBytes[:10]) }},
		{"internal/crc.CheckCCITT", "CSBK", 0, func() { ok = crc.CheckCCITT(csbkBytes[:10], csbkCRC) }},
		{"internal/crc.CRC32", "d-sms message", 0, func() { u32 = crc.CRC32(message) }},
		{"internal/crc.CheckCRC32", "d-sms message", 0, func() { ok = crc.CheckCRC32(message, msgCRC) }},
		{"internal/crc.Checksum5", "embedded LC", 0, func() { u8 = crc.Checksum5(embLC) }},
		{"internal/crc.CheckChecksum5", "embedded LC", 0, func() { ok = crc.CheckChecksum5(embLC, embCS) }},
		{"internal/crc.CRC7", "reverse channel", 0, func() { u8 = crc.CRC7(rcInfoBytes, 7) }},
		{"internal/crc.CheckCRC7", "reverse channel", 0, func() { ok = crc.CheckCRC7(rcInfoBytes, 7, 0x2A) }},
		{"internal/crc.CRC8", "", 0, func() { u8 = crc.CRC8(lcBytes[:], 72) }},
		{"internal/crc.CheckCRC8", "", 0, func() { ok = crc.CheckCRC8(lcBytes[:], 72, 0x5A) }},
		{"internal/crc.CRC9", "rate 1/2 block", 0, func() { u16 = crc.CRC9(rate12Bytes, 80) }},
		{"internal/crc.CheckCRC9", "rate 1/2 block", 0, func() { ok = crc.CheckCRC9(rate12Bytes, 80, uint16(bit.Uint(rcCRCBits[:]))) }},
	}
	_ = ambeFrame
	_, _, _, _, _, _, _, _, _, _, _, _ = bits96, bits196, rate34Out, trellisOut, bits77, bits36, bits11, bits20, bits16, bits7, frags, cachOut
	_, _, _, _, _, _, _, _, _, _, _, _ = bits32, rsData, rsCW, u16, u32, u8, ok, r, err, ber, n, reserved
	return out
}
