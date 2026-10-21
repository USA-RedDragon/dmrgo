package lrrp_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp"
)

var message lrrp.Message

// ExampleDecodeMessageInto reads the position report of the example of the
// MOTOTRBO specification.
func ExampleDecodeMessageInto() {
	data, _ := hex.DecodeString("071a22042468ace0341f4dbc778051118ecd8d118ad47b00636c0006")
	if err := lrrp.DecodeMessageInto(&message, data); err != nil {
		fmt.Println(err)
		return
	}
	r := &message.Report
	fmt.Printf("%s %x %d-%02d-%02d %02d:%02d %.6f %.6f %.2f m\n", message.Document, r.RequestID,
		r.Time.Year, r.Time.Month, r.Time.Day, r.Time.Hour, r.Time.Minute, r.Circle2D.Latitude, r.Circle2D.Longitude, r.Circle2D.Radius)
	// Output:
	// Immediate Location Report 2468ace0 2003-06-30 07:30 12.345345 24.668866 0.77 m
}

// ExampleMessage_AppendBinary builds a request for a position every 60
// seconds.
func ExampleMessage_AppendBinary() {
	m := &lrrp.Message{Document: lrrp.TriggeredRequest}
	m.Request.RequestID, m.Request.HasRequestID = []byte{0x24, 0x68, 0xac, 0xe0}, true
	m.Request.Periodic = true
	m.Request.Interval, m.Request.HasInterval = 60, true
	out, _ := m.AppendBinary(nil)
	fmt.Printf("%x\n", out)
	// Output:
	// 090922042468ace034313c
}
