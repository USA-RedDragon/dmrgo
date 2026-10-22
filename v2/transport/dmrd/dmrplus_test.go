package dmrd_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

func TestDMRPlusOptionsExample(t *testing.T) {
	t.Parallel()

	const text = "StartRef=4013;RelinkTime=15;UserLink=1;TS1_1=262;TS1_2=1;TS1_3=20;TS1_4=110;TS1_5=270;"
	o, err := dmrd.ParseDMRPlusOptions([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	set := func(v uint32) dmrd.OptionValue { return dmrd.OptionValue{State: dmrd.OptionSet, Value: v} }
	want := dmrd.DMRPlusOptions{StartRef: set(4013), RelinkTime: set(15), UserLink: set(1), TS1: [5]dmrd.OptionValue{set(262), set(1), set(20), set(110), set(270)}}
	if o != want {
		t.Fatalf("%+v", o)
	}
	if got, err := o.AppendText(nil); err != nil || string(got) != text {
		t.Fatalf("%q", got)
	}
}

func TestDMRPlusOptionsClearAndUnknown(t *testing.T) {
	t.Parallel()

	o, err := dmrd.ParseDMRPlusOptions([]byte(" StartRef = ; Foo=1;TS1_3=7;junk;TS1_9=2"))
	if err != nil {
		t.Fatal(err)
	}
	if o.StartRef.State != dmrd.OptionClear || o.TS1[2] != (dmrd.OptionValue{State: dmrd.OptionSet, Value: 7}) || o.UserLink.State != dmrd.OptionAbsent {
		t.Fatalf("%+v", o)
	}
	if got, err := o.AppendText([]byte("x")); err != nil || string(got) != "xStartRef=;TS1_3=7;" {
		t.Fatalf("%q", got)
	}
	for _, bad := range []string{"StartRef=12a", "UserLink=4294967296"} {
		if _, err := dmrd.ParseDMRPlusOptions([]byte(bad)); !errors.Is(err, dmrd.ErrText) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}
