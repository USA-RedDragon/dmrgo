package csbk_test

import (
	"errors"
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// Example sends an OACSU call request from a Requester to a
// Responder and reads the answer back.
func Example() {
	q, _ := csbk.NewRequester(&csbk.RequesterConfig{ColorCode: 1, Source: layer2.SyncSourceMS})
	r, _ := csbk.NewResponder(&csbk.ResponderConfig{Address: 1002, ColorCode: 1, Source: layer2.SyncSourceMS})
	out := make([]csbk.Output, 0, csbk.MaxOutputs)

	if err := q.RequestOACSU(0, 1002, 1001, layer3.ServiceOptions{}, nil); err != nil {
		panic(err)
	}
	out = q.Tick(0, out[:0])
	request := out[0].Burst

	out = r.Burst(0, &request, layer2.Hint{}, out[:0])
	fmt.Println(out[0].Kind, out[0].Service, "from", out[0].Source)
	at, _ := r.Deadline()
	out = r.Tick(at, out[:0])
	answer := out[0].Burst

	out = q.Burst(at+30*time.Millisecond, &answer, layer2.Hint{}, out[:0])
	fmt.Println(out[0].Kind, out[0].Result)
	// Output:
	// Request OACSU from 1001
	// Done Answered
}

// ExampleRequester_RequestRadioCheck sends a radio check with 5 preambles
// and no retries, and reports NoAnswer when no radio replies.
func ExampleRequester_RequestRadioCheck() {
	q, _ := csbk.NewRequester(&csbk.RequesterConfig{ColorCode: 7, Source: layer2.SyncSourceMS, Retries: -1})
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
	q, _ := csbk.NewRequester(&csbk.RequesterConfig{Source: layer2.SyncSourceMS})
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
			fmt.Println(out[i].Kind, "to", out[i].Target, "until", out[i].Until)
		}
	}
	// Output:
	// Announced to 3191868 until 120ms
	// Joined to 3191868 until 0s
}

// ExampleWakeup sends BS_Dwn_Act to wake a repeater and reports
// NoAnswer when the repeater does not reply.
func ExampleWakeup() {
	w, _ := csbk.NewWakeup(&csbk.WakeupConfig{ColorCode: 7})
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
	_, err := csbk.NewResponder(&csbk.ResponderConfig{})
	var ce *dmrgo.ConfigError
	if errors.As(err, &ce) {
		fmt.Println(ce.Field)
	}
	// Output:
	// Address
}
