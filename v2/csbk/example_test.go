package csbk_test

import (
	"errors"
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// Example sends an OACSU call request from a Requester to a
// Responder and reads the answer back.
func Example() {
	q, _ := csbk.NewRequester(&csbk.RequesterConfig{Slot: enums.Slot1, ColorCode: 1, Source: layer2.SyncSourceMS})
	r, _ := csbk.NewResponder(&csbk.ResponderConfig{Slot: enums.Slot1, Address: 1002, ColorCode: 1, Source: layer2.SyncSourceMS})
	out := make([]csbk.Output, 0, csbk.MaxOutputs)

	if err := q.RequestOACSU(0, 1002, 1001, layer3.ServiceOptions{}, nil); err != nil {
		panic(err)
	}
	out = q.Tick(0, out[:0])
	request := out[0].Burst

	out = r.Burst(out[0].Due, &request, layer2.Hint{}, out[:0])
	fmt.Println(out[0].Kind, out[0].Service, "from", out[0].Call.Source)
	at, _ := r.Deadline()
	out = r.Tick(at, out[:0])
	answer, due := out[0].Burst, out[0].Due

	out = q.Burst(due, &answer, layer2.Hint{}, out[:0])
	fmt.Println(out[0].Kind, out[0].Result, "at", due)
	// Output:
	// Request OACSU from 1001
	// Done Answered at 60ms
}

// ExampleRequester_RequestRadioCheck sends a radio check with 5 preambles
// and no retries, and reports NoAnswer when no radio replies.
func ExampleRequester_RequestRadioCheck() {
	q, _ := csbk.NewRequester(&csbk.RequesterConfig{Slot: enums.Slot1, ColorCode: 7, Source: layer2.SyncSourceMS, Retries: -1})
	if err := q.RequestRadioCheck(0, 3140598, 3191868, &csbk.RequestOptions{Preambles: 5}); err != nil {
		panic(err)
	}
	out := make([]csbk.Output, 0, csbk.MaxOutputs)
	bursts := 0
	for {
		at, ok := q.Deadline()
		if !ok {
			break
		}
		out = q.Tick(at, out[:0])
		for i := range out {
			if out[i].Kind == csbk.OutputBurst {
				bursts++
			} else {
				fmt.Println(bursts, "bursts,", out[i].Result, "at", at)
			}
		}
	}
	// Output:
	// 6 bursts, NoAnswer at 2.3s
}

// ExamplePreambles follows the Pre_CSBKs of a call alert and
// reports the end of the announced blocks, then the CSBK that they announce.
func ExamplePreambles() {
	q, _ := csbk.NewRequester(&csbk.RequesterConfig{Slot: enums.Slot1, Source: layer2.SyncSourceMS})
	_ = q.RequestCallAlert(0, 3191868, 3140598, &csbk.RequestOptions{Preambles: 2})
	p, err := csbk.NewPreambles(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	out := make([]csbk.Output, 0, csbk.MaxOutputs)
	for range 3 {
		at, _ := q.Deadline()
		sent := q.Tick(at, nil)
		out = p.Burst(at, &sent[0].Burst, layer2.Hint{}, out[:0])
		for i := range out {
			fmt.Println(out[i].Kind, "to", out[i].Call.Target, "until", out[i].Until)
		}
	}
	// Output:
	// Announced to 3191868 until 120ms
	// Joined to 3191868 until 0s
}

// ExampleWakeup sends BS_Dwn_Act to wake a repeater and reports
// NoAnswer when the repeater does not reply.
func ExampleWakeup() {
	w, _ := csbk.NewWakeup(&csbk.WakeupConfig{Slot: enums.Slot1, ColorCode: 7})
	_ = w.Wake(0, 314926, 3191868)
	for {
		at, ok := w.Deadline()
		if !ok {
			break
		}
		for _, o := range w.Tick(at, nil) {
			if o.Kind == csbk.OutputDone {
				fmt.Println(at, o.Kind, o.Result)
			} else {
				fmt.Println(at, o.Kind)
			}
		}
	}
	// Output:
	// 0s Burst
	// 360ms Burst
	// 720ms Done NoAnswer
}

// ExampleNewResponder_configError shows the error of a config field out of
// range.
func ExampleNewResponder_configError() {
	_, err := csbk.NewResponder(&csbk.ResponderConfig{Slot: enums.Slot1})
	var ce *dmrgo.ConfigError
	if errors.As(err, &ce) {
		fmt.Println(ce.Field)
	}
	// Output:
	// Address
}

// ExampleResponder_callAlert answers a Motorola radio check and a call alert from
// radio 3140598 to radio 3191868 on a direct mode channel, and the
// Requester reports each answer.
func ExampleResponder_callAlert() {
	q, _ := csbk.NewRequester(&csbk.RequesterConfig{Slot: enums.Slot1, ColorCode: 1, Source: layer2.SyncSourceMS})
	r, _ := csbk.NewResponder(&csbk.ResponderConfig{Slot: enums.Slot1, Address: 3191868, ColorCode: 1, Source: layer2.SyncSourceMS})
	out := make([]csbk.Output, 0, csbk.MaxOutputs)
	now := time.Duration(0)
	for i := range 2 {
		var err error
		if i == 0 {
			err = q.RequestRadioCheck(now, 3191868, 3140598, nil)
		} else {
			err = q.RequestCallAlert(now, 3191868, 3140598, nil)
		}
		if err != nil {
			fmt.Println(err)
			return
		}
		out = q.Tick(now, out[:0])
		request := out[0]

		out = r.Burst(request.Due, &request.Burst, request.Hint, out[:0])
		fmt.Println("responder:", out[0].Kind, out[0].Service, "from", out[0].Call.Source)
		at, _ := r.Deadline()
		out = r.Tick(at, out[:0])
		answer := out[0]

		out = q.Burst(answer.Due, &answer.Burst, answer.Hint, out[:0])
		fmt.Println("requester:", out[0].Kind, out[0].Service, out[0].Result, "after", answer.Due-request.Due)
		now = answer.Due + time.Second
	}
	// Output:
	// responder: Request RadioCheck from 3140598
	// requester: Done RadioCheck Answered after 60ms
	// responder: Request CallAlert from 3140598
	// requester: Done CallAlert Answered after 60ms
}

// ExampleResponder answers a Motorola radio check on the aligned channel
// timing of a BS and on the offset one, and prints when each answer goes on
// air.
func ExampleResponder() {
	q, _ := csbk.NewRequester(&csbk.RequesterConfig{Slot: enums.Slot1, ColorCode: 1, Source: layer2.SyncSourceMS})
	if err := q.RequestRadioCheck(0, 3140598, 3191868, nil); err != nil {
		fmt.Println(err)
		return
	}
	sent := q.Tick(0, nil)
	request := sent[0].Burst
	for _, offset := range []bool{false, true} {
		r, err := csbk.NewResponder(&csbk.ResponderConfig{Slot: enums.Slot1, Address: 3140598, ColorCode: 1, Source: layer2.SyncSourceMS, Offset: offset})
		if err != nil {
			fmt.Println(err)
			return
		}
		out := make([]csbk.Output, 0, csbk.MaxOutputs)
		for _, o := range r.Burst(sent[0].Due, &request, layer2.Hint{}, out[:0]) {
			fmt.Println(o.Kind, o.Service, "from", o.Call.Source, "state", r.State())
		}
		at, _ := r.Deadline()
		for _, o := range r.Tick(at, out[:0]) {
			fmt.Println("offset", offset, o.Kind, "due", o.Due, "state", r.State())
		}
	}
	// Output:
	// Request RadioCheck from 3191868 state Sending
	// offset false Burst due 60ms state Idle
	// Request RadioCheck from 3191868 state Sending
	// offset true Burst due 90ms state Idle
}
