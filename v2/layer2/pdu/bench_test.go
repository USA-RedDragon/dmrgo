package pdu_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// benchPayloads returns 64 random 12-octet payloads, sealed for dt when dt
// has a CRC or RS(12,9) parity.
func benchPayloads(dt enums.DataType) [][12]byte {
	r := rand.New(rand.NewSource(int64(dt) + 7))
	out := make([][12]byte, 64)
	for i := range out {
		r.Read(out[i][:])
		_ = pdu.Seal(&out[i], dt)
	}
	return out
}

func benchBits(dt enums.DataType) [][96]bit.Bit {
	p := benchPayloads(dt)
	out := make([][96]bit.Bit, len(p))
	for i := range p {
		out[i] = bit.Unpack96(p[i])
	}
	return out
}

func BenchmarkPDUDecode(b *testing.B) {
	csbk := benchBits(enums.DataTypeCSBK)
	lc := benchBits(enums.DataTypeVoiceLCHeader)
	pi := benchBits(enums.DataTypePIHeader)
	dh := benchBits(enums.DataTypeDataHeader)
	mbc := benchBits(enums.DataTypeMBCHeader)
	usbd := benchBits(enums.DataTypeUnifiedSingleBlock)
	r12 := benchBits(enums.DataTypeRate12)
	var csbkDst pdu.CSBK
	var csbkP pdu.CSBKPayloads
	var lcDst pdu.FullLinkControl
	var lcP pdu.FullLinkControlPayloads
	var dhDst pdu.DataHeader
	var dhP pdu.DataHeaderPayloads
	var mbcDst pdu.MBCHeader
	var mbcP pdu.MBCHeaderPayloads
	cases := []struct {
		name string
		fn   func(i int)
	}{
		{"CSBK", func(i int) { _ = pdu.DecodeCSBK(csbk[i]) }},
		{"CSBKInto", func(i int) { pdu.DecodeCSBKInto(&csbkDst, &csbkP, csbk[i]) }},
		{"FullLinkControl", func(i int) { _ = pdu.DecodeFullLinkControl(lc[i], enums.DataTypeVoiceLCHeader) }},
		{"FullLinkControlInto", func(i int) {
			pdu.DecodeFullLinkControlInto(&lcDst, &lcP, lc[i], enums.DataTypeVoiceLCHeader)
		}},
		{"PIHeader", func(i int) { _ = pdu.DecodePIHeader(pi[i]) }},
		{"DataHeaderInto", func(i int) { pdu.DecodeDataHeaderInto(&dhDst, &dhP, dh[i]) }},
		{"MBCHeaderInto", func(i int) { pdu.DecodeMBCHeaderInto(&mbcDst, &mbcP, mbc[i]) }},
		{"MBCContinuation", func(i int) { _ = pdu.DecodeMBCContinuation(mbc[i]) }},
		{"UnifiedSingleBlockData", func(i int) { _ = pdu.DecodeUnifiedSingleBlockData(usbd[i]) }},
		{"Rate12Data", func(i int) { _ = pdu.DecodeRate12Data(r12[i]) }},
		{"SlotType", func(i int) { _ = pdu.DecodeSlotType([20]bit.Bit(r12[i][:20])) }},
		{"EmbeddedSignalling", func(i int) { _ = pdu.DecodeEmbeddedSignalling([16]bit.Bit(r12[i][:16])) }},
		{"TACT", func(i int) { _ = pdu.DecodeTACT([7]bit.Bit(r12[i][:7])) }},
		{"ShortLC", func(i int) { _ = pdu.DecodeShortLC([36]bit.Bit(r12[i][:36])) }},
		{"ServiceOptions", func(i int) { _ = layer3.DecodeServiceOptions([8]bit.Bit(r12[i][:8])) }},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				c.fn(i % 64)
				i++
			}
		})
	}
}

func BenchmarkPDUBytes(b *testing.B) {
	csbk := benchPayloads(enums.DataTypeCSBK)
	lc := benchPayloads(enums.DataTypeVoiceLCHeader)
	var csbkDst pdu.CSBK
	var csbkP pdu.CSBKPayloads
	var lcDst pdu.FullLinkControl
	var lcP pdu.FullLinkControlPayloads
	csbkPDU := pdu.DecodeCSBKBytes(csbk[0])
	lcPDU := pdu.DecodeFullLinkControl(bit.Unpack96(lc[0]), enums.DataTypeVoiceLCHeader)
	cases := []struct {
		name string
		fn   func(i int)
	}{
		{"DecodeCSBKBytes", func(i int) { _ = pdu.DecodeCSBKBytes(csbk[i]) }},
		{"DecodeCSBKBytesInto", func(i int) { pdu.DecodeCSBKBytesInto(&csbkDst, &csbkP, csbk[i]) }},
		{"CSBKBytes", func(int) { _, _ = csbkPDU.Bytes() }},
		{"DecodeFullLCCodeword", func(i int) { _, _, _ = pdu.DecodeFullLCCodeword(lc[i], enums.DataTypeVoiceLCHeader) }},
		{"DecodeFullLinkControlBytesInto", func(i int) {
			_ = pdu.DecodeFullLinkControlBytesInto(&lcDst, &lcP, [9]byte(lc[i][:9]))
		}},
		{"FullLinkControlCodeword", func(int) { _, _ = lcPDU.Codeword() }},
		{"Seal", func(i int) { _ = pdu.Seal(&csbk[i], enums.DataTypeCSBK) }},
		{"BlockCRCOK", func(i int) { _ = pdu.BlockCRCOK(csbk[i], enums.DataTypeCSBK) }},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				c.fn(i % 64)
				i++
			}
		})
	}
}

func BenchmarkPDUEncode(b *testing.B) {
	csbk := benchBits(enums.DataTypeCSBK)
	lc := benchBits(enums.DataTypeVoiceLCHeader)
	pi := benchBits(enums.DataTypePIHeader)
	dh := benchBits(enums.DataTypeDataHeader)
	mbc := benchBits(enums.DataTypeMBCHeader)
	usbd := benchBits(enums.DataTypeUnifiedSingleBlock)
	r12 := benchBits(enums.DataTypeRate12)
	csbks := make([]pdu.CSBK, 64)
	lcs := make([]pdu.FullLinkControl, 64)
	pis := make([]pdu.PIHeader, 64)
	dhs := make([]pdu.DataHeader, 64)
	mbcs := make([]pdu.MBCHeader, 64)
	usbds := make([]pdu.UnifiedSingleBlockData, 64)
	r12s := make([]pdu.Rate12Data, 64)
	sts := make([]pdu.SlotType, 64)
	slcs := make([]pdu.ShortLC, 64)
	for i := range 64 {
		csbks[i] = pdu.DecodeCSBK(csbk[i])
		lcs[i] = pdu.DecodeFullLinkControl(lc[i], enums.DataTypeVoiceLCHeader)
		pis[i] = pdu.DecodePIHeader(pi[i])
		dhs[i] = pdu.DecodeDataHeader(dh[i])
		mbcs[i] = pdu.DecodeMBCHeader(mbc[i])
		usbds[i] = pdu.DecodeUnifiedSingleBlockData(usbd[i])
		r12s[i] = pdu.DecodeRate12Data(r12[i])
		sts[i] = pdu.SlotType{ColorCode: enums.ColorCode(i % 16), DataType: enums.DataType(i % 10)}
		slcs[i] = pdu.DecodeShortLC([36]bit.Bit(r12[i][:36]))
	}
	cases := []struct {
		name string
		fn   func(i int)
	}{
		{"CSBK", func(i int) { _, _ = csbks[i].Encode() }},
		{"FullLinkControl", func(i int) { _, _ = lcs[i].Encode() }},
		{"PIHeader", func(i int) { _, _ = pis[i].Encode() }},
		{"DataHeader", func(i int) { _, _ = dhs[i].Encode() }},
		{"MBCHeader", func(i int) { _, _ = mbcs[i].Encode() }},
		{"UnifiedSingleBlockData", func(i int) { _, _ = usbds[i].Encode() }},
		{"Rate12Data", func(i int) { _, _ = r12s[i].Encode() }},
		{"SlotType", func(i int) { _, _ = sts[i].Encode() }},
		{"ShortLC", func(i int) { _, _ = slcs[i].Encode() }},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				c.fn(i % 64)
				i++
			}
		})
	}
}
