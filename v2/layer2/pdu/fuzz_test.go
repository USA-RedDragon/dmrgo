package pdu_test

import (
	"math/bits"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func captureBursts(tb testing.TB) []layer2.RawBurst {
	tb.Helper()
	files, err := filepath.Glob("../testdata/*.bin")
	if err != nil || len(files) == 0 {
		tb.Fatalf("no captures: %v", err)
	}
	var out []layer2.RawBurst
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			tb.Fatal(err)
		}
		for i := 0; i+33 <= len(data); i += 33 {
			out = append(out, layer2.RawBurst(data[i:i+33]))
		}
	}
	return out
}

func isDataSync(p enums.SyncPattern) bool {
	switch p {
	case enums.SyncBSSourcedData, enums.SyncMSSourcedData, enums.SyncTDMA1Data, enums.SyncTDMA2Data:
		return true
	default:
		return false
	}
}

func fecOf(v any) fec.Result {
	f := reflect.ValueOf(v).Elem().FieldByName("FEC")
	if !f.IsValid() {
		return fec.Result{}
	}
	r, _ := f.Interface().(fec.Result)
	return r
}

func clearFEC(v any) {
	if f := reflect.ValueOf(v).Elem().FieldByName("FEC"); f.IsValid() {
		f.Set(reflect.Zero(f.Type()))
	}
}

func checkCodec[B comparable, T any](t *testing.T, name string, in B, dec func(B) T, enc func(*T) (B, error), str func(*T) string) B {
	t.Helper()
	v := dec(in)
	_ = str(&v)
	out, err := enc(&v)
	if err != nil {
		t.Fatalf("%s: Encode(Decode(%v)): %v", name, in, err)
	}
	w := dec(out)
	if r := fecOf(&w); r.HasErrors() {
		t.Fatalf("%s: Decode(Encode(Decode(%v))) = %v has FEC %+v", name, in, out, r)
	}
	again, err := enc(&w)
	if err != nil || again != out {
		t.Fatalf("%s: encode not stable for %v: %v then %v (%v)", name, in, out, again, err)
	}
	clearFEC(&v)
	clearFEC(&w)
	if !reflect.DeepEqual(v, w) {
		t.Fatalf("%s: Decode(Encode(p)) != p for p = Decode(%v)\np    %s\ngot  %s", name, in, str(&v), str(&w))
	}
	return out
}

func checkInto[B any, T any, P any](t *testing.T, name string, in, other B, dec func(B) T, into func(*T, *P, B)) {
	t.Helper()
	var dst T
	var p P
	into(&dst, &p, other)
	into(&dst, &p, in)
	want := dec(in)
	if !reflect.DeepEqual(dst, want) {
		t.Fatalf("%s: Into over a used destination differs from Decode for %v", name, in)
	}
	if n := testing.AllocsPerRun(1, func() { into(&dst, &p, in) }); n != 0 {
		t.Fatalf("%s: Into allocates %v times", name, n)
	}
}

func invert(b [12]byte) [12]byte {
	for i := range b {
		b[i] = ^b[i]
	}
	return b
}

var bptcKinds = [...]enums.DataType{
	enums.DataTypeVoiceLCHeader,
	enums.DataTypeTerminatorWithLC,
	enums.DataTypeCSBK,
	enums.DataTypeMBCHeader,
	enums.DataTypeMBCContinuation,
	enums.DataTypeDataHeader,
	enums.DataTypeUnifiedSingleBlock,
	enums.DataTypePIHeader,
	enums.DataTypeRate12,
	enums.DataTypeIdle,
	enums.DataTypeMBCContinuation,
	enums.DataTypeMBCContinuation,
	enums.DataTypeMBCContinuation,
	enums.DataTypeMBCContinuation,
}

const sealFlag = 0x80

func FuzzBPTCPDU(f *testing.F) {
	bptcSeeds(f, func(kind uint8, data []byte) { f.Add(kind, data) })
	f.Fuzz(fuzzBPTCPDU)
}

func bptcSeeds(tb testing.TB, add func(kind uint8, data []byte)) {
	for _, b := range captureBursts(tb) {
		if p, _ := b.Sync(); !isDataSync(p) {
			continue
		}
		st := b.SlotType()
		payload, _ := b.BPTCPayload()
		for i, dt := range bptcKinds {
			if dt == st.DataType {
				add(uint8(i), payload[:])
			}
		}
	}
	prfill := pdu.PRFillBits()
	add(uint8(9), prfill[:])
	for op := range byte(64) {
		for _, k := range []uint8{0, 2, 3} {
			add(k|sealFlag, []byte{op, 0, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE})
		}
		add(uint8(2)|sealFlag, []byte{op, byte(enums.FIDMotorola), 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE})
	}
	for k := range uint8(len(bptcKinds)) {
		add(k|sealFlag, []byte{0x85, 0x00, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0, 0, 0, 0xFF, k})
	}
	for format := range byte(16) {
		add(uint8(5)|sealFlag, []byte{0x40 | format, 0x01, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0})
		add(uint8(5)|sealFlag, []byte{format, 0x00, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0})
	}
}

func fuzzBPTCPDU(t *testing.T, kind uint8, data []byte) {
	var in [12]byte
	copy(in[:], data)
	k := int(kind&^sealFlag) % len(bptcKinds)
	dt := bptcKinds[k]
	sealed := kind&sealFlag != 0 && sealBPTC(t, &in, dt)
	var out [12]byte
	switch k {
	case 0, 1:
		out = checkCodec(t, dt.String(), in,
			func(b [12]byte) pdu.FullLinkControl { return pdu.DecodeFullLinkControl(b, dt) },
			(*pdu.FullLinkControl).Encode, (*pdu.FullLinkControl).String)
		checkInto(t, dt.String(), in, invert(in),
			func(b [12]byte) pdu.FullLinkControl { return pdu.DecodeFullLinkControl(b, dt) },
			func(d *pdu.FullLinkControl, p *pdu.FullLinkControlPayloads, b [12]byte) {
				pdu.DecodeFullLinkControlInto(d, p, b, dt)
			})
	case 2:
		out = checkCodec(t, "CSBK", in, pdu.DecodeCSBK, (*pdu.CSBK).Encode, (*pdu.CSBK).String)
		checkInto(t, "CSBK", in, invert(in), pdu.DecodeCSBK, pdu.DecodeCSBKInto)
		checkCSBKViews(t, out)
	case 3:
		out = checkCodec(t, "MBCHeader", in, pdu.DecodeMBCHeader, (*pdu.MBCHeader).Encode, (*pdu.MBCHeader).String)
		checkInto(t, "MBCHeader", in, invert(in), pdu.DecodeMBCHeader, pdu.DecodeMBCHeaderInto)
	case 4:
		out = checkCodec(t, "MBCContinuation", in, pdu.DecodeMBCContinuation, (*pdu.MBCContinuation).Encode, (*pdu.MBCContinuation).String)
		checkMBCViews(t, in, out)
	case 5:
		out = checkCodec(t, "DataHeader", in, pdu.DecodeDataHeader, (*pdu.DataHeader).Encode, (*pdu.DataHeader).String)
		checkInto(t, "DataHeader", in, invert(in), pdu.DecodeDataHeader, pdu.DecodeDataHeaderInto)
	case 6:
		out = checkCodec(t, "USBD", in, pdu.DecodeUnifiedSingleBlockData, (*pdu.UnifiedSingleBlockData).Encode, (*pdu.UnifiedSingleBlockData).String)
	case 7:
		out = checkCodec(t, "PIHeader", in, pdu.DecodePIHeader, (*pdu.PIHeader).Encode, (*pdu.PIHeader).String)
	case 8:
		out = checkCodec(t, "Rate12Data", in, pdu.DecodeRate12Data, (*pdu.Rate12Data).Encode, (*pdu.Rate12Data).String)
		checkConfirmed(t, in)
	case 9:
		checkFullLCBytes(t, [9]byte(in[:9]))
		if pdu.IsPRFill(in) != (in == pdu.PRFillBits()) {
			t.Fatalf("IsPRFill(%x) = %t", in, pdu.IsPRFill(in))
		}
		return
	case 10:
		out = checkCodec(t, "CGAPContinuation", in, pdu.DecodeCGAPContinuation, (*pdu.CGAPContinuation).Encode, (*pdu.CGAPContinuation).String)
	case 11:
		out = checkCodec(t, "MVAPContinuation", in, pdu.DecodeMVAPContinuation, (*pdu.MVAPContinuation).Encode, (*pdu.MVAPContinuation).String)
	case 12:
		out = checkCodec(t, "BCAPContinuation", in, pdu.DecodeBCAPContinuation, (*pdu.BCAPContinuation).Encode, (*pdu.BCAPContinuation).String)
	case 13:
		out = checkCodec(t, "VNAPContinuation", in, pdu.DecodeVNAPContinuation, (*pdu.VNAPContinuation).Encode, (*pdu.VNAPContinuation).String)
	}
	if sealed && out != in {
		t.Fatalf("%s: Encode(Decode(%x)) = %x drops bits", dt, in, out)
	}
	if sealed && len(data) > 13 && data[12] != 0 {
		checkOctetError(t, k, in, int(data[13])%len(in), data[12])
	}
}

func sealBPTC(t *testing.T, p *[12]byte, dt enums.DataType) bool {
	t.Helper()
	switch {
	case dt == enums.DataTypeMBCContinuation:
		blocks := [][12]byte{*p}
		pdu.SetMBCLastBlockCRC(blocks)
		*p = blocks[0]
		if !pdu.MBCLastBlockCRCOK(*p) {
			t.Fatalf("sealed MBC continuation %x fails its CRC", *p)
		}
	case pdu.Seal(p, dt) != nil:
		return dt == enums.DataTypeRate12
	case dt.CarriesFullLC():
		if !pdu.FullLCParityOK(*p, dt) {
			t.Fatalf("%s: sealed %x fails its parity", dt, *p)
		}
	case !pdu.BlockCRCOK(*p, dt):
		t.Fatalf("%s: sealed %x fails its CRC", dt, *p)
	}
	return true
}

func checkMBCViews(t *testing.T, in, out [12]byte) {
	t.Helper()
	c := pdu.DecodeMBCContinuation(in)
	cgap, mvap, bcap, vnap := c.AsCGAP(), c.AsMVAP(), c.AsBCAP(), c.AsVNAP()
	views := [...][12]byte{must(cgap.Encode()), must(mvap.Encode()), must(bcap.Encode()), must(vnap.Encode())}
	crcs := [...]fec.Result{cgap.FEC, mvap.FEC, bcap.FEC, vnap.FEC}
	crcOK := pdu.MBCLastBlockCRCOK(out)
	for i, v := range views {
		if [10]byte(v[:10]) != [10]byte(out[:10]) || crcs[i].Uncorrectable == crcOK || crcOK && v != out {
			t.Fatalf("MBCContinuation view %d of %x encodes as %x with %+v, block %x", i, in, v, crcs[i], out)
		}
	}
}

func checkOctetError(t *testing.T, k int, in [12]byte, at int, e byte) {
	t.Helper()
	bad := in
	bad[at] ^= e
	dt := bptcKinds[k]
	var r fec.Result
	switch k {
	case 0, 1:
		got, want := pdu.DecodeFullLinkControl(bad, dt), pdu.DecodeFullLinkControl(in, dt)
		if got.FEC != (fec.Result{BitsChecked: 96, ErrorsCorrected: bits.OnesCount8(e)}) {
			t.Fatalf("%s: %x with error %02x in octet %d gives %+v", dt, in, e, at, got.FEC)
		}
		got.FEC, want.FEC = fec.Result{}, fec.Result{}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: %x with error %02x in octet %d not corrected", dt, in, e, at)
		}
		return
	case 2:
		r = pdu.DecodeCSBK(bad).FEC
	case 3:
		r = pdu.DecodeMBCHeader(bad).FEC
	case 4:
		r.Uncorrectable = !pdu.MBCLastBlockCRCOK(bad)
	case 5:
		r = pdu.DecodeDataHeader(bad).FEC
	case 6:
		r = pdu.DecodeUnifiedSingleBlockData(bad).FEC
	case 7:
		r = pdu.DecodePIHeader(bad).FEC
	case 10:
		r = pdu.DecodeCGAPContinuation(bad).FEC
	case 11:
		r = pdu.DecodeMVAPContinuation(bad).FEC
	case 12:
		r = pdu.DecodeBCAPContinuation(bad).FEC
	case 13:
		r = pdu.DecodeVNAPContinuation(bad).FEC
	default:
		return
	}
	if !r.Uncorrectable {
		t.Fatalf("%s kind %d: %x with error %02x in octet %d passes its CRC", dt, k, in, e, at)
	}
}

func checkCSBKViews(t *testing.T, b [12]byte) {
	t.Helper()
	c := pdu.DecodeCSBK(b)
	views := 0
	set := func(name string, ok bool, setView func(*pdu.CSBK) error) {
		if !ok {
			return
		}
		views++
		e := c
		if err := setView(&e); err != nil {
			t.Fatalf("CSBK %x: %s view does not set back: %v", b, name, err)
		}
		if got := must(e.Encode()); got != b {
			t.Fatalf("CSBK %x: %s view sets back as %x", b, name, got)
		}
	}
	bs, ok := c.AsBSOutboundActivation()
	set("BS_Dwn_Act", ok, func(e *pdu.CSBK) error { return e.SetBSOutboundActivation(bs) })
	mi, ok := c.AsTalkgroupDataGrantMultiItem()
	set("TD_GRANT_MI", ok, func(e *pdu.CSBK) error { return e.SetTalkgroupDataGrantMultiItem(mi) })
	rc, ok := c.AsMotorolaRadioCheck()
	set("radio check", ok, func(e *pdu.CSBK) error { return e.SetMotorolaRadioCheck(rc) })
	ca, ok := c.AsMotorolaCallAlert()
	set("call alert", ok, func(e *pdu.CSBK) error { return e.SetMotorolaCallAlert(ca) })
	ack, ok := c.AsMotorolaCallAlertAck()
	set("call alert ack", ok, func(e *pdu.CSBK) error { return e.SetMotorolaCallAlertAck(ack) })
	_, _, _, _, _ = bs.String(), mi.String(), rc.String(), ca.String(), ack.String()
	if (views == 2) != (c.Opcode == pdu.CSBKBSOutboundActivation && c.FID == enums.FIDStandardized) {
		t.Fatalf("CSBK %x has %d payload views", b, views)
	}
}

func checkConfirmed(t *testing.T, in [12]byte) {
	t.Helper()
	r := pdu.DecodeRate12Data(in)
	dbsn, user := r.ConfirmedDBSN(), r.ConfirmedUserData()
	if err := r.SetConfirmed(dbsn|0x80, user); err == nil {
		t.Fatal("SetConfirmed accepts an 8-bit DBSN")
	}
	if r.Data != in {
		t.Fatal("a refused SetConfirmed changed the block")
	}
	noErr(r.SetConfirmed(dbsn, user))
	if !r.ConfirmedCRC9OK() || r.ConfirmedDBSN() != dbsn || r.ConfirmedUserData() != user {
		t.Fatalf("confirmed block of %x does not read back", in)
	}
}

func checkFullLCBytes(t *testing.T, lc [9]byte) {
	t.Helper()
	v, err := pdu.DecodeFullLinkControlBytes(lc)
	var into pdu.FullLinkControl
	var p pdu.FullLinkControlPayloads
	if intoErr := pdu.DecodeFullLinkControlBytesInto(&into, &p, lc); (intoErr == nil) != (err == nil) || !reflect.DeepEqual(into, v) {
		t.Fatalf("DecodeFullLinkControlBytesInto(%x) differs from DecodeFullLinkControlBytes", lc)
	}
	if err != nil {
		return
	}
	_ = v.String()
	b, err := v.Bytes()
	if err != nil {
		t.Fatalf("Bytes(DecodeFullLinkControlBytes(%x)): %v", lc, err)
	}
	if b != lc {
		t.Fatalf("DecodeFullLinkControlBytes(%x).Bytes() = %x", lc, b)
	}
	for _, dt := range []enums.DataType{enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC} {
		cw := must(pdu.LCBytes(lc).Codeword(dt))
		got, res, err := pdu.DecodeFullLCCodeword(cw, dt)
		if err != nil || res.HasErrors() || got != lc {
			t.Fatalf("%s: DecodeFullLCCodeword(Codeword(%x)) = %x %+v %v", dt, lc, got, res, err)
		}
		w := pdu.DecodeFullLinkControl(cw, dt)
		w.FEC = fec.Result{}
		v.FEC = fec.Result{}
		w.BurstDataType = v.BurstDataType
		if !reflect.DeepEqual(v, w) {
			t.Fatalf("%s: DecodeFullLinkControlBytes(%x) differs from DecodeFullLinkControl", dt, lc)
		}
	}
}
