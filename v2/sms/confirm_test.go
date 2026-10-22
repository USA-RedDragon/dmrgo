package sms_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

func sendText(t *testing.T, text *sms.Text, confirmed bool) (data.Result, int) {
	t.Helper()
	var h sms.Header
	payload, err := sms.AppendPacket(nil, text, confirmed, &h)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := data.NewSender(&data.SenderConfig{Source: layer2.SyncSourceMS})
	if err != nil {
		t.Fatal(err)
	}
	rx, err := data.NewReceiver(&data.ReceiverConfig{Address: text.Destination, Respond: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Send(0, &data.Message{Header: h.Data, Rate: enums.DataTypeRate12, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	outputs := make([]data.Output, 0, data.MaxOutputs)
	events := make([]data.Event, 0, data.MaxEvents)
	var replies []data.Event
	delivered := 0
	for now := time.Duration(0); now < 10*time.Second; now += 10 * time.Millisecond {
		for _, e := range replies {
			for _, o := range tx.Burst(e.Due, &e.Burst, layer2.Hint{}, outputs[:0]) {
				if o.Kind == data.OutputDone {
					return o.Result, delivered
				}
			}
		}
		replies = replies[:0]
		for _, o := range tx.Tick(now, outputs[:0]) {
			if o.Kind == data.OutputDone {
				return o.Result, delivered
			}
			for _, e := range rx.Burst(o.Due, &o.Burst, layer2.Hint{}, events[:0]) {
				switch e.Kind {
				case data.EventDelivered:
					delivered++
				case data.EventRespond:
					replies = append(replies, e)
				case data.EventCallStart, data.EventResponse, data.EventCallEnd, data.EventReassembled:
				}
			}
		}
	}
	t.Fatal("the Sender did not finish")
	return 0, 0
}

func TestConfirmedShortDataWithData(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		text      sms.Text
		confirmed bool
		want      data.Result
	}{
		{sms.Text{Format: sms.FormatDefinedData, DD: enums.DDUTF16LE, Source: 1, Destination: 2, Body: []byte("hi")}, true, data.ResultDelivered},
		{sms.Text{Format: sms.FormatDefinedData, DD: enums.DDUTF16LE, Source: 1, Destination: 2, Body: []byte("hi")}, false, data.ResultSent},
		{sms.Text{Format: sms.FormatHytera, Source: 1, Destination: 2, Body: []byte("hi")}, true, data.ResultDelivered},
		{sms.Text{Format: sms.FormatIP, Source: 1, Destination: 2, Body: []byte("hi")}, true, data.ResultDelivered},
	} {
		got, delivered := sendText(t, &tt.text, tt.confirmed)
		if got != tt.want || delivered != 1 {
			t.Errorf("%v confirmed %t: %v, delivered %d, want %v", tt.text.Format, tt.confirmed, got, delivered, tt.want)
		}
	}
}
