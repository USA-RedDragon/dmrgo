package csbk_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestColorCodeFilter(t *testing.T) {
	t.Parallel()

	withCC := func(raw layer2.RawBurst, cc enums.ColorCode) layer2.RawBurst {
		t.Helper()
		p, res := raw.BPTCPayload()
		if res.Uncorrectable {
			t.Fatal("payload")
		}
		b, err := layer2.BuildDataBurst(p, enums.DataTypeCSBK, cc, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	decoded := func(raw layer2.RawBurst) *layer2.Burst {
		t.Helper()
		b := layer2.NewBurst()
		if err := b.DecodeFromBytesWith(raw, &layer2.DecodeOptions{}); err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, tt := range []struct {
		name   string
		own    enums.ColorCode
		rx     enums.ColorCode
		answer bool
	}{
		{"same color code", 7, 7, true},
		{"other color code", 7, 1, false},
		{"zero accepts any", 0, 1, true},
	} {
		for _, path := range []string{"Burst", "Decoded"} {
			q := mustRequester(t, &csbk.RequesterConfig{Slot: enums.Slot1, ColorCode: tt.own, Source: layer2.SyncSourceMS})
			r := mustResponder(t, &csbk.ResponderConfig{Slot: enums.Slot1, Address: 3002198, ColorCode: tt.own, Source: layer2.SyncSourceMS})
			if err := q.RequestRadioCheck(0, 3002198, 3002068, nil); err != nil {
				t.Fatal(err)
			}
			sent := q.Tick(0, nil)
			request := withCC(sent[0].Burst, tt.rx)
			var out []csbk.Output
			if path == "Burst" {
				out = r.Burst(sent[0].Due, &request, layer2.Hint{}, nil)
			} else {
				out = r.Decoded(sent[0].Due, decoded(request), layer2.Hint{}, nil)
			}
			if got := len(out) == 1 && out[0].Kind == csbk.OutputRequest; got != tt.answer {
				t.Errorf("%s, Responder.%s: request taken %t, want %t", tt.name, path, got, tt.answer)
			}

			reply := mustResponder(t, &csbk.ResponderConfig{Slot: enums.Slot1, Address: 3002198, Source: layer2.SyncSourceMS})
			own := withCC(sent[0].Burst, tt.own)
			reply.Burst(sent[0].Due, &own, layer2.Hint{}, nil)
			at, _ := reply.Deadline()
			answer := reply.Tick(at, nil)
			raw := withCC(answer[0].Burst, tt.rx)
			if path == "Burst" {
				out = q.Burst(answer[0].Due, &raw, layer2.Hint{}, nil)
			} else {
				out = q.Decoded(answer[0].Due, decoded(raw), layer2.Hint{}, nil)
			}
			if got := len(out) == 1 && out[0].Result == csbk.ResultAnswered; got != tt.answer {
				t.Errorf("%s, Requester.%s: answer taken %t, want %t", tt.name, path, got, tt.answer)
			}
		}
	}
}
