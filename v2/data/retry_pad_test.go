package data_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

func TestReceiverRetryKeepsPayloadLength(t *testing.T) {
	t.Parallel()

	txt := sms.Text{Format: sms.FormatIP, Source: 3191868, Destination: 3140598, Confirmed: true, Body: []byte("hello private")}
	var hd sms.Header
	payload, err := sms.AppendPacket(nil, &txt, &hd)
	if err != nil {
		t.Fatal(err)
	}
	s, err := data.NewSender(&data.SenderConfig{ColorCode: 1, Access: enums.AccessImpolite, Slot: enums.Slot2, ResponseWait: time.Second, Retries: 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(0, &data.Message{Header: hd.Data, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	r, err := data.NewReceiver(&data.ReceiverConfig{Lost: -1})
	if err != nil {
		t.Fatal(err)
	}
	tries := 0
	for now := time.Duration(0); now < 20*time.Second; now += 20 * time.Millisecond {
		for _, o := range s.Tick(now, nil) {
			if o.Kind != data.OutputBurst {
				continue
			}
			raw := o.Burst
			for _, e := range r.Burst(now, &raw, layer2.Hint{Slot: enums.Slot2}, nil) {
				if e.Kind != data.EventDelivered {
					continue
				}
				tries++
				if !bytes.Equal(e.Message.Payload, payload) {
					t.Fatalf("try %d: payload %x, want %x", o.Try, e.Message.Payload, payload)
				}
			}
		}
	}
	if tries != 3 {
		t.Fatalf("%d deliveries, want 3", tries)
	}
}
