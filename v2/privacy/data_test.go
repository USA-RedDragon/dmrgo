package privacy_test

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/privacy"
)

type dataCapture struct {
	key          [5]byte
	header       [12]byte
	phead        [12]byte
	blocks       [][12]byte
	plain, clear []byte
}

func loadDataCapture(t *testing.T) *dataCapture {
	t.Helper()
	f, err := os.Open("testdata/data-rc4.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	c := &dataCapture{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			t.Fatalf("bad line %q", sc.Text())
		}
		b, err := hex.DecodeString(fields[1])
		if err != nil {
			t.Fatal(err)
		}
		switch fields[0] {
		case "key":
			c.key = [5]byte(b)
		case "header":
			c.header = [12]byte(b)
		case "phead":
			c.phead = [12]byte(b)
		case "block":
			c.blocks = append(c.blocks, [12]byte(b))
		case "plain":
			c.plain = b
		case "clear":
			c.clear = b
		default:
			t.Fatalf("bad line %q", sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return c
}

func receiveData(t *testing.T, c *dataCapture) data.Message {
	t.Helper()
	rx, err := data.NewReceiver(nil)
	if err != nil {
		t.Fatal(err)
	}
	type burst struct {
		p  [12]byte
		dt enums.DataType
	}
	bursts := make([]burst, 0, 2+len(c.blocks))
	bursts = append(bursts, burst{c.header, enums.DataTypeDataHeader}, burst{c.phead, enums.DataTypeDataHeader})
	for _, b := range c.blocks {
		bursts = append(bursts, burst{b, enums.DataTypeRate12})
	}
	events := make([]data.Event, 0, data.MaxEvents)
	var now time.Duration
	for _, b := range bursts {
		raw, err := layer2.BuildDataBurst(b.p, b.dt, 1, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		now += 60 * time.Millisecond
		events = rx.Burst(now, &raw, layer2.Hint{}, events[:0])
		for i := range events {
			if events[i].Kind == data.EventDelivered {
				m := events[i].Message
				m.Payload = bytes.Clone(m.Payload)
				if m.Proprietary != nil {
					p := *m.Proprietary
					m.Proprietary = &p
				}
				return m
			}
		}
	}
	t.Fatal("no packet delivered")
	return data.Message{}
}

func TestDataCaptureHeaders(t *testing.T) {
	t.Parallel()

	c := loadDataCapture(t)
	for _, p := range [][12]byte{c.header, c.phead} {
		if h := pdu.DecodeDataHeader(p); h.FEC.Uncorrectable {
			t.Errorf("header %X: CRC fails", p)
		}
	}
	h := pdu.DecodeDataHeader(c.phead)
	ph, ok := h.AsProprietary()
	if !ok {
		t.Fatal("the second header is not a P_HEAD")
	}
	want := privacy.Info{Encrypted: true, Header: true, MFID: enums.FIDMotorola, Algorithm: privacy.AlgorithmARC4, KeyID: 1, MI: 0x0C01A418}
	if got := privacy.FromProprietaryHeader(&ph); got != want {
		t.Errorf("%+v, want %+v", got, want)
	}
}

func TestReceiverDecryptsDataCapture(t *testing.T) {
	t.Parallel()

	c := loadDataCapture(t)
	m := receiveData(t, c)
	if m.BadCRC || m.Proprietary == nil {
		t.Fatalf("delivered %+v", m)
	}
	cipher := bytes.Clone(m.Payload)

	none := privacy.NewReceiver()
	if in := none.Data(&m); in.Decrypting || !in.Encrypted || in.KeyID != 1 || in.Algorithm != privacy.AlgorithmARC4 {
		t.Errorf("without a key: %+v", in)
	}
	if !bytes.Equal(m.Payload, cipher) {
		t.Error("a Receiver without the key changed the payload")
	}

	prx := privacy.NewReceiver()
	if err := prx.AddKey(privacy.NewARC4Key(1, c.key)); err != nil {
		t.Fatal(err)
	}
	in := prx.Data(&m)
	if !in.Decrypting || in.MI != 0x0C01A418 {
		t.Errorf("info %+v", in)
	}
	if !bytes.Equal(m.Payload, c.plain) {
		t.Fatalf("payload %X, want %X", m.Payload, c.plain)
	}
	if !bytes.Equal(m.Payload[1:], c.clear[1:]) {
		t.Errorf("payload %X differs from the clear packet %X beyond the IPv4 identification", m.Payload, c.clear)
	}
	var raw [9]byte
	copy(raw[:], m.Payload)
	hdr := pdu.DecodeUDPIPv4CompressedHeader(raw)
	if hdr.Octets() != 5 {
		t.Fatalf("compressed header %+v", hdr)
	}
	text := m.Payload[hdr.Octets():]
	units := make([]uint16, 0, len(text)/2)
	for i := 0; i+1 < len(text); i += 2 {
		if u := binary.BigEndian.Uint16(text[i:]); u != 0 {
			units = append(units, u)
		}
	}
	if s := string(utf16.Decode(units)); s != "Test Message Hytera to DSDFME" {
		t.Errorf("text %q", s)
	}
}

func TestDataOtherHeaders(t *testing.T) {
	t.Parallel()

	if privacy.FromProprietaryHeader(nil) != (privacy.Info{}) {
		t.Error("nil")
	}
	for _, h := range []pdu.ProprietaryHeader{
		{MFID: 0x68, ManufacturerData: [8]byte{0x01, 0x01}},
		{MFID: enums.FIDMotorola, ManufacturerData: [8]byte{0x10}},
	} {
		if got := privacy.FromProprietaryHeader(&h); got != (privacy.Info{}) {
			t.Errorf("%+v: %+v", h, got)
		}
	}
	h := pdu.ProprietaryHeader{MFID: enums.FIDMotorola, ManufacturerData: [8]byte{0x51, 7, 0, 0, 1, 2, 3, 4}}
	if got := privacy.FromProprietaryHeader(&h); got.Algorithm != privacy.AlgorithmAES256 || got.KeyID != 7 || got.MI != 0x01020304 {
		t.Errorf("AES-256: %+v", got)
	}
	k, err := privacy.NewAESKey(7, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	r := privacy.NewReceiver()
	if err := r.AddKey(k); err != nil {
		t.Fatal(err)
	}
	m := data.Message{Proprietary: &h, Payload: []byte{1, 2, 3}}
	if in := r.Data(&m); in.Decrypting || !bytes.Equal(m.Payload, []byte{1, 2, 3}) {
		t.Errorf("AES data, which has no proven framing: %+v %X", in, m.Payload)
	}
	h.ManufacturerData[0] = 0x01
	if got := privacy.FromProprietaryHeader(&h); got.Algorithm != privacy.AlgorithmBasic {
		t.Errorf("Basic: %+v", got)
	}
	var nr *privacy.Receiver
	if nr.Data(&m) != (privacy.Info{}) || r.Data(nil) != (privacy.Info{}) {
		t.Error("nil")
	}
}

func TestDataCaptureHeaderVariants(t *testing.T) {
	t.Parallel()

	for _, v := range []struct {
		hex  string
		want privacy.Info
	}{
		{"4F10110700001533909F76EF", privacy.Info{Encrypted: true, Header: true, MFID: enums.FIDMotorola, Algorithm: privacy.AlgorithmARC4, KeyID: 7, MI: 0x1533909F}},
		{"4F1011010000 18A039331CA5", privacy.Info{Encrypted: true, Header: true, MFID: enums.FIDMotorola, Algorithm: privacy.AlgorithmARC4, KeyID: 1, MI: 0x18A03933}},
		{"1F10020111F7320D18238F80", privacy.Info{}},
		{"3F68010100000000 0000E671", privacy.Info{}},
	} {
		b, err := hex.DecodeString(strings.ReplaceAll(v.hex, " ", ""))
		if err != nil {
			t.Fatal(err)
		}
		h := pdu.DecodeDataHeader([12]byte(b))
		ph, ok := h.AsProprietary()
		if h.FEC.Uncorrectable || !ok {
			t.Fatalf("%s: CRC fails or not a P_HEAD", v.hex)
		}
		if got := privacy.FromProprietaryHeader(&ph); got != v.want {
			t.Errorf("%s: %+v, want %+v", v.hex, got, v.want)
		}
	}
}
