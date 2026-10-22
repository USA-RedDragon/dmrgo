package packet_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

var anyToneDD = []struct {
	name       string
	header     string
	block      string
	payload    []byte
	bitPadding uint8
	crc9       uint16
}{
	{"A", "200e368173e9e06d93cbdc27ddad5d7f77fd757e6898fd5e7120228e94da253c65",
		"2d2f2222223762b2722222285e1d5d7f77fd7577d6df2f22222e76e27222222659",
		[]byte{0, 0, 'A', 0, 0, 0}, 48, 0x083},
	{"AB", "204d368172e9e17d911bdce7ddad5d7f77fd757e6890fd6e7230222e8c5a343c75",
		"282fe22222d9e2327a22221d5e1d5d7f77fd7577d7ef2f722220695272f2222919",
		[]byte{0, 0, 'A', 0, 'B', 0, 0, 0}, 32, 0x117},
}

func anyToneBurst(t *testing.T, s string) layer2.RawBurst {
	t.Helper()
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != 33 {
		t.Fatalf("burst %s: %v", s, err)
	}
	return layer2.RawBurst(raw)
}

func TestAnyToneDefinedData(t *testing.T) {
	t.Parallel()

	for _, tt := range anyToneDD {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			air := []layer2.RawBurst{anyToneBurst(t, tt.header), anyToneBurst(t, tt.block)}
			a := packet.NewAssembler(1)
			var b layer2.Burst
			var msg *packet.Message
			for i, raw := range air {
				if err := b.DecodeFromBytes(raw); err != nil {
					t.Fatalf("burst %d: %v", i, err)
				}
				if i == 1 {
					blk, ok := b.Data.(*pdu.Rate34Data)
					if !ok || blk.ConfirmedDBSN() != 0 || blk.ConfirmedCRC9() != tt.crc9 || !blk.ConfirmedCRC9OK() {
						t.Fatalf("block %T is not a confirmed block with DBSN 0 and CRC-9 %03x", b.Data, tt.crc9)
					}
				}
				m, err := addBurst(a, &b)
				if err != nil {
					t.Fatalf("burst %d: %v", i, err)
				}
				if m != nil {
					msg = m
				}
			}
			if msg == nil {
				t.Fatal("no message")
			}
			d := get(t, &msg.Header, (*pdu.DataHeader).AsDefinedData)
			if !d.ResponseRequested || d.SARQ || !d.FullMessageFlag || d.AppendedBlocks != 1 || d.BitPadding != tt.bitPadding {
				t.Fatalf("header %+v", d)
			}
			if msg.BadCRC || msg.DataType != enums.DataTypeRate34 || !bytes.Equal(msg.Data, tt.payload) {
				t.Fatalf("data %x, bad CRC %v, rate %v", msg.Data, msg.BadCRC, msg.DataType)
			}

			h := header(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{
				ResponseRequested: true, SAP: enums.SAPShortData, LLIDDestination: 3140598, LLIDSource: 3191868,
				DefinedData: enums.DDFormat(1), FullMessageFlag: true})
			tx := packet.NewBuilder(1)
			p, err := tx.Segment(h, tt.payload, enums.DataTypeRate34)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Retry(&p, nil); !errors.Is(err, packet.ErrFormat) {
				t.Errorf("Retry without SARQ: %v, want ErrFormat", err)
			}
			got, err := packet.Bursts(&p, 7, layer2.SyncSourceMS)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(air) {
				t.Fatalf("%d bursts, want %d", len(got), len(air))
			}
			for i := range air {
				if got[i] != air[i] {
					t.Errorf("burst %d: %x, want %x", i, got[i][:], air[i][:])
				}
			}
		})
	}
}
