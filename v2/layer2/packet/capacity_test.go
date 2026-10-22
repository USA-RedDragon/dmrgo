package packet_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
)

func TestCapacity(t *testing.T) {
	t.Parallel()

	data := payload(30)
	p, err := packet.NewPacket(confirmed(), data, enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	n := len(p.Blocks)
	if _, err := packet.NewBuilder(n-1).Segment(confirmed(), data, enums.DataTypeRate12); !errors.Is(err, packet.ErrNoRoom) {
		t.Errorf("Segment of %d blocks into a Builder for %d: %v, want ErrNoRoom", n, n-1, err)
	}
	var zero packet.Builder
	if _, err := zero.Segment(confirmed(), data, enums.DataTypeRate12); !errors.Is(err, packet.ErrNoRoom) {
		t.Errorf("Segment into the zero Builder: %v, want ErrNoRoom", err)
	}
	if _, err := packet.NewBuilder(n).Segment(confirmed(), data, enums.DataTypeRate12); err != nil {
		t.Errorf("Segment of %d blocks into a Builder for %d: %v", n, n, err)
	}

	if _, _, err := packet.NewAssembler(n - 1).Add(&p.Header); !errors.Is(err, packet.ErrNoRoom) {
		t.Errorf("header of %d blocks into an Assembler for %d: %v, want ErrNoRoom", n, n-1, err)
	}
	var none packet.Assembler
	if _, _, err := none.Add(&p.Header); !errors.Is(err, packet.ErrNoRoom) {
		t.Errorf("header into the zero Assembler: %v, want ErrNoRoom", err)
	}
	m, err := feed(t, packet.NewAssembler(n), p)
	if err != nil || m == nil || !bytes.Equal(m.Data, data) {
		t.Fatalf("packet of %d blocks into an Assembler for %d: %v", n, n, err)
	}
	if m.Blocks != n || m.DataType != enums.DataTypeRate12 {
		t.Errorf("Blocks %d DataType %v, want %d Rate12", m.Blocks, m.DataType, n)
	}
	ack, err := packet.NewResponse(confirmed().ConfirmedDataHeader, packet.ResponseACK, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m, err := feed(t, &none, ack); err != nil || m == nil || m.Blocks != 0 {
		t.Errorf("ACK into the zero Assembler: %+v %v", m, err)
	}
}
