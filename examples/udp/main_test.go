package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestRun(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := run(&buf, 5, 1); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"| screen | Rate 1/2 | 0 | 5/5 |",
		"| fields+zstd+dict | Rate 1 | 0 | 5/5 |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestCleanChannel(t *testing.T) {
	t.Parallel()
	tm := serviceOrder()
	dict := trainDictionary(&tm, dictSeed)
	for _, rate := range rates() {
		for e := range numEncodings {
			r, err := runCell(cell{rate: rate, enc: e, forms: 10, seed: 3}, &tm, dict)
			if err != nil {
				t.Fatalf("%v %v: %v", rate, e, err)
			}
			if r.tx.acked != 10 || r.rx.delivered != 10 || r.rx.copies != 0 || r.tx.resends != 0 || r.link.tries != 10 {
				t.Errorf("%v %v: acked %d delivered %d copies %d resends %d tries %d", rate, e,
					r.tx.acked, r.rx.delivered, r.rx.copies, r.tx.resends, r.link.tries)
			}
		}
	}
}

func TestNoisyChannelDeliversOnce(t *testing.T) {
	t.Parallel()
	tm := serviceOrder()
	dict := trainDictionary(&tm, dictSeed)
	r, err := runCell(cell{rate: enums.DataTypeRate34, ber: 0.02, enc: encFieldsDict, forms: 20, seed: 5}, &tm, dict)
	if err != nil {
		t.Fatal(err)
	}
	if r.tx.acked != r.tx.submitted || r.rx.delivered != r.tx.submitted {
		t.Errorf("acked %d and delivered %d of %d", r.tx.acked, r.rx.delivered, r.tx.submitted)
	}
	if r.link.tries <= r.link.datagrams {
		t.Errorf("tries %d for %d datagrams: the channel made no errors", r.link.tries, r.link.datagrams)
	}
}

func TestDictionaryMismatch(t *testing.T) {
	t.Parallel()
	tm := serviceOrder()
	a, err := newCodec(1, trainDictionary(&tm, 1))
	if err != nil {
		t.Fatal(err)
	}
	defer a.close()
	b, err := newCodec(2, trainDictionary(&tm, 2))
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	f := randomForm(newRand(9), &tm)
	body := a.encode(nil, encFieldsDict, &f)
	if _, err := a.decode(encFieldsDict, &tm, body); err != nil {
		t.Fatalf("same dictionary: %v", err)
	}
	if _, err := b.decode(encFieldsDict, &tm, body); err == nil {
		t.Error("another dictionary decoded the form")
	}
}
