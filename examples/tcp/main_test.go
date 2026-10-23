package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestCleanSession(t *testing.T) {
	t.Parallel()
	for _, rate := range rates() {
		r, err := runCell(cell{rate: rate, window: 1, mtu: 576, cycles: 2, seed: 1})
		if err != nil {
			t.Fatalf("%v: %v", rate, err)
		}
		if !r.done || r.transferred != r.payload || len(r.submit) != 2 || len(r.back) != 2 {
			t.Fatalf("%v: done %v, %d of %d octets, %d submits, %d backs", rate, r.done, r.transferred, r.payload, len(r.submit), len(r.back))
		}
		if r.links[0].failed+r.links[1].failed != 0 {
			t.Errorf("%v: %d DMR packets failed on a clean channel", rate, r.links[0].failed+r.links[1].failed)
		}
	}
}

func TestNoisySessionDeliversEveryOctet(t *testing.T) {
	t.Parallel()
	r, err := runCell(cell{rate: enums.DataTypeRate12, ber: 0.02, window: 1, mtu: 576, cycles: 1, seed: 2})
	if err != nil {
		t.Fatal(err)
	}
	if r.transferred != r.payload {
		t.Errorf("%d of %d octets", r.transferred, r.payload)
	}
}

func TestRepeatable(t *testing.T) {
	t.Parallel()
	var a, b bytes.Buffer
	run(&a, 1, 3, 576)
	run(&b, 1, 3, 576)
	if a.String() != b.String() {
		t.Errorf("two runs differ:\n%s\n%s", a.String(), b.String())
	}
	if !strings.Contains(a.String(), "| simplex | 1 | Rate 1/2 | 0 |") {
		t.Errorf("output lacks the clean rate 1/2 row:\n%s", a.String())
	}
}

func TestFormInputAddressesTheInputFields(t *testing.T) {
	t.Parallel()
	in := formInput(newRand(1))
	f := formFields()[0]
	a := f.row*cols + f.col + len(f.label) + 2
	if want := []byte{orderSBA, addressCodes[a>>6], addressCodes[a&0x3F]}; !bytes.Equal(in[3:6], want) {
		t.Errorf("first SBA % x, want % x", in[3:6], want)
	}
}
