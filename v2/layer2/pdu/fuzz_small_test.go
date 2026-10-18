package pdu_test

import (
	"math/bits"
	"reflect"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// The kinds of FuzzSmallPDU.
const (
	smallEMB = iota
	smallSlotType
	smallShortLC
	smallTACT
	smallReverseChannel
	smallUDPIPv4
	smallRate34
	smallRate1
	smallGPS
	smallEMBCodeword
	smallSlotTypeCodeword
	smallKinds
)

// FuzzSmallPDU checks the decoders and encoders of the PDUs that are not a
// 12-octet BPTC payload on any input, as FuzzBPTCPDU does: the EMB, slot
// type, Short LC, TACT, RC, UDP/IPv4 compressed header, Rate 3/4 and Rate 1
// data blocks and GPS Info, and the EMB and slot type codeword decoders and
// setters.
func FuzzSmallPDU(f *testing.F) {
	smallSeeds(f, func(kind uint8, data []byte) { f.Add(kind, data) })
	f.Fuzz(fuzzSmallPDU)
}

// smallSeeds calls add with each seed of FuzzSmallPDU.
func smallSeeds(tb testing.TB, add func(kind uint8, data []byte)) {
	for _, b := range captureBursts(tb) {
		p, _ := b.Sync()
		switch {
		case p == enums.SyncNone:
			emb := b.EMB()
			cw := must(emb.Encode())
			add(uint8(smallEMB), cw[:])
			add(uint8(smallEMBCodeword), cw[:])
		case isDataSync(p):
			st := b.SlotType()
			cw := must(st.Encode())
			add(uint8(smallSlotType), cw[:])
			add(uint8(smallSlotTypeCodeword), cw[:])
			switch st.DataType {
			case enums.DataTypeRate34:
				d, _ := b.Rate34Payload()
				add(uint8(smallRate34), d[:])
			case enums.DataTypeRate1:
				d, _ := b.Rate1Payload()
				add(uint8(smallRate1), d[:])
			default:
			}
		}
	}
	for slco := range byte(16) {
		add(uint8(smallShortLC), []byte{slco<<4 | 0x05, 0xA5, 0x5A, 0x3C, 0x00, 0x96})
	}
	for k := range smallKinds {
		add(uint8(k), []byte{0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0, 0x0F})
	}
}

// fuzzSmallPDU is the body of FuzzSmallPDU.
func fuzzSmallPDU(t *testing.T, kind uint8, data []byte) {
	var in [24]byte
	copy(in[:], data)
	switch kind % smallKinds {
	case smallEMB:
		checkCodec(t, "EmbeddedSignalling", [2]byte(in[:2]), pdu.DecodeEmbeddedSignalling, (*pdu.EmbeddedSignalling).Encode, (*pdu.EmbeddedSignalling).String)
	case smallSlotType:
		checkCodec(t, "SlotType", [3]byte(in[:3]), pdu.DecodeSlotType, (*pdu.SlotType).Encode, (*pdu.SlotType).String)
	case smallShortLC:
		b := [5]byte(in[:5])
		checkCodec(t, "ShortLC", b, pdu.DecodeShortLC, (*pdu.ShortLC).Encode, (*pdu.ShortLC).String)
		checkInto(t, "ShortLC", b, [5]byte(in[5:10]), pdu.DecodeShortLC, pdu.DecodeShortLCInto)
	case smallTACT:
		checkCodec(t, "TACT", [1]byte(in[:1]), pdu.DecodeTACT, (*pdu.TACT).Encode, (*pdu.TACT).String)
	case smallReverseChannel:
		checkCodec(t, "ReverseChannel", [2]byte(in[:2]), pdu.DecodeReverseChannel, (*pdu.ReverseChannel).Encode, (*pdu.ReverseChannel).String)
	case smallUDPIPv4:
		checkCodec(t, "UDPIPv4CompressedHeader", [9]byte(in[:9]), pdu.DecodeUDPIPv4CompressedHeader, (*pdu.UDPIPv4CompressedHeader).Encode, (*pdu.UDPIPv4CompressedHeader).String)
	case smallRate34:
		checkRate34(t, [18]byte(in[:18]))
	case smallRate1:
		checkRate1(t, in)
	case smallGPS:
		checkCodec(t, "FLCGPSInfo", [7]byte(in[:7]), pdu.DecodeFLCGPSInfo, (*pdu.FLCGPSInfo).Encode, (*pdu.FLCGPSInfo).String)
	case smallEMBCodeword:
		checkEMBCodeword(t, uint16(in[0])<<8|uint16(in[1]), in[2])
	case smallSlotTypeCodeword:
		checkSlotTypeCodeword(t, uint32(in[0])<<16|uint32(in[1])<<8|uint32(in[2]), in[3], in[4])
	}
}

// checkEMBCodeword fails t unless DecodeEMBCodeword of cw corrects to a
// codeword at the distance that it reports, the info decodes back from its
// own codeword, and the setters of EMBInfo take the fields of v.
func checkEMBCodeword(t *testing.T, cw uint16, v byte) {
	t.Helper()
	info, res := pdu.DecodeEMBCodeword(cw)
	if info&1 != 0 {
		t.Fatalf("DecodeEMBCodeword(%04x) = %02x sets bit 0", cw, info)
	}
	if got, r := pdu.DecodeEMBCodeword(info.Codeword()); got != info || r.HasErrors() {
		t.Fatalf("EMB info %02x does not decode back from its codeword: %02x %+v", info, got, r)
	}
	if d := bits.OnesCount16(cw ^ info.Codeword()); !res.Uncorrectable && d != res.ErrorsCorrected {
		t.Fatalf("DecodeEMBCodeword(%04x) corrects %d bits, reports %d", cw, d, res.ErrorsCorrected)
	}
	es := info.EmbeddedSignalling()
	if back, err := es.Info(); err != nil || back != info {
		t.Fatalf("EMB info %02x through EmbeddedSignalling gives %02x %v", info, back, err)
	}
	e := info
	cc, lcss, pi := enums.ColorCode(v>>4), enums.LCSS(v&3), v&4 != 0
	if e.SetColorCode(cc|16) == nil || e.SetLCSS(lcss|4) == nil || e != info {
		t.Fatal("EMBInfo setters accept values wider than their fields or change e")
	}
	noErr(e.SetColorCode(cc))
	noErr(e.SetLCSS(lcss))
	e.SetPI(pi)
	if e.ColorCode() != cc || e.LCSS() != lcss || e.PI() != pi || e&1 != 0 {
		t.Fatalf("EMBInfo %02x does not read back color code %d, LCSS %d, PI %t", e, cc, lcss, pi)
	}
}

// checkSlotTypeCodeword fails t unless DecodeSlotTypeCodeword of the low 20
// bits of cw corrects to a codeword at the distance that it reports, the
// info decodes back from its own codeword, and the setters of SlotTypeInfo
// take the color code cc and the data type dt.
func checkSlotTypeCodeword(t *testing.T, cw uint32, cc, dt byte) {
	t.Helper()
	info, res := pdu.DecodeSlotTypeCodeword(cw)
	if got, r := pdu.DecodeSlotTypeCodeword(info.Codeword()); got != info || r.HasErrors() {
		t.Fatalf("slot type info %02x does not decode back from its codeword: %02x %+v", info, got, r)
	}
	if d := bits.OnesCount32((cw ^ info.Codeword()) & 0xFFFFF); !res.Uncorrectable && d != res.ErrorsCorrected {
		t.Fatalf("DecodeSlotTypeCodeword(%05x) corrects %d bits, reports %d", cw, d, res.ErrorsCorrected)
	}
	st := info.SlotType()
	if back, err := st.Info(); err != nil || back != info {
		t.Fatalf("slot type info %02x through SlotType gives %02x %v", info, back, err)
	}
	s := info
	errCC, errDT := s.SetColorCode(enums.ColorCode(cc)), s.SetDataType(enums.DataType(dt))
	if (errCC == nil) != (cc < 16) || (errDT == nil) != (dt < 16) {
		t.Fatalf("SlotTypeInfo setters: color code %d gives %v, data type %d gives %v", cc, errCC, dt, errDT)
	}
	want := info
	if cc < 16 {
		want = want&0x0F | pdu.SlotTypeInfo(cc<<4)
	}
	if dt < 16 {
		want = want&0xF0 | pdu.SlotTypeInfo(dt)
	}
	if s != want || !reflect.DeepEqual(s.SlotType(), pdu.SlotType{ColorCode: s.ColorCode(), DataType: s.DataType()}) {
		t.Fatalf("SlotTypeInfo %02x after setting %d and %d, want %02x", s, cc, dt, want)
	}
}

// checkRate34 fails t unless the Rate 3/4 block b keeps every bit through
// Decode and Encode and a confirmed block made from its DBSN and user data
// reads them back and passes its CRC-9.
func checkRate34(t *testing.T, b [18]byte) {
	t.Helper()
	if out := checkCodec(t, "Rate34Data", b, pdu.DecodeRate34Data, (*pdu.Rate34Data).Encode, (*pdu.Rate34Data).String); out != b {
		t.Fatalf("Rate34Data drops bits of %x", b)
	}
	r := pdu.DecodeRate34Data(b)
	dbsn, user := r.ConfirmedDBSN(), r.ConfirmedUserData()
	if r.SetConfirmed(dbsn|0x80, user) == nil || r.Data != b {
		t.Fatal("SetConfirmed accepts an 8-bit DBSN or changes the block")
	}
	noErr(r.SetConfirmed(dbsn, user))
	if !r.ConfirmedCRC9OK() || r.ConfirmedDBSN() != dbsn || r.ConfirmedUserData() != user {
		t.Fatalf("confirmed Rate 3/4 block of %x does not read back", b)
	}
}

// checkRate1 runs the checks of checkRate34 on the Rate 1 block b.
func checkRate1(t *testing.T, b [24]byte) {
	t.Helper()
	if out := checkCodec(t, "Rate1Data", b, pdu.DecodeRate1Data, (*pdu.Rate1Data).Encode, (*pdu.Rate1Data).String); out != b {
		t.Fatalf("Rate1Data drops bits of %x", b)
	}
	r := pdu.DecodeRate1Data(b)
	dbsn, user := r.ConfirmedDBSN(), r.ConfirmedUserData()
	if r.SetConfirmed(dbsn|0x80, user) == nil || r.Data != b {
		t.Fatal("SetConfirmed accepts an 8-bit DBSN or changes the block")
	}
	noErr(r.SetConfirmed(dbsn, user))
	if !r.ConfirmedCRC9OK() || r.ConfirmedDBSN() != dbsn || r.ConfirmedUserData() != user {
		t.Fatalf("confirmed Rate 1 block of %x does not read back", b)
	}
}
