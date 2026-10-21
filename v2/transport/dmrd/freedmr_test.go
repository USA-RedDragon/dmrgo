package dmrd_test

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

func sameOptions(a, b dmrd.FreeDMROptions) bool {
	a.Key, a.Lang, b.Key, b.Lang = nil, nil, nil, nil
	return reflect.DeepEqual(a, b)
}

func set(v uint32) dmrd.OptionValue { return dmrd.OptionValue{State: dmrd.OptionSet, Value: v} }

func TestFreeDMROptionsCheatSheet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text     string
		want     dmrd.FreeDMROptions
		key      string
		ts1, ts2 []uint32
	}{
		{
			text: "KEY=secret;TS1=91,92;TS2=235;DEFAULT_DIAL_TS1=4400;DEFAULT_DIAL_TS2=2350;TIMER=15",
			want: dmrd.FreeDMROptions{DialTS1: set(4400), DialTS2: set(2350), Timer: set(15)},
			key:  "secret", ts1: []uint32{91, 92}, ts2: []uint32{235},
		},
		{
			text: "KEY=secret;TS1=91,92;TS2=235;DIAL1=4400;DIAL2=2350;TGTO1=10;TGTO2=10;DIALTO1=30;DIALTO2=30;NDD_SLOT=1",
			want: dmrd.FreeDMROptions{DialTS1: set(4400), DialTS2: set(2350), TS1TGTimer: set(10), TS2TGTimer: set(10), TS1DialTimer: set(30), TS2DialTimer: set(30), DirectDialSlot: set(1)},
			key:  "secret", ts1: []uint32{91, 92}, ts2: []uint32{235},
		},
		{
			text: "TS1=;TS2=;DEFAULT_DIAL_TS2=2350;TIMER=15",
			want: dmrd.FreeDMROptions{DialTS2: set(2350), Timer: set(15)},
		},
		{
			text: "KEY=my-secret-key;TS1=91;TS2=235;DIALTG=1;DYNAMIC=0;TIMER=15",
			want: dmrd.FreeDMROptions{DialATG: set(1), Dynamic: set(0), Timer: set(15)},
			key:  "my-secret-key", ts1: []uint32{91}, ts2: []uint32{235},
		},
		{
			text: "DIAL=;TIMER=1",
			want: dmrd.FreeDMROptions{DialTS2: dmrd.OptionValue{State: dmrd.OptionClear}, Timer: set(1)},
		},
		{
			text: "TG_TIMER=10;DIAL_TIMER=30;TS1_TIMER=5;TS2Timer=6;IDENT=0;SINGLE=1;LANG=en_GB",
			want: dmrd.FreeDMROptions{TGTimer: set(10), DialTimer: set(30), TS1Timer: set(5), TS2Timer: set(6), Voice: set(0), Single: set(1)},
		},
	}
	for _, tt := range tests {
		o, err := dmrd.ParseFreeDMROptions([]byte(tt.text))
		if err != nil {
			t.Errorf("%q: %v", tt.text, err)
			continue
		}
		if string(o.Key) != tt.key {
			t.Errorf("%q: Key %q, want %q", tt.text, o.Key, tt.key)
		}
		o.Key, o.Lang = nil, nil
		if !sameOptions(o, tt.want) {
			t.Errorf("%q:\n got %+v\nwant %+v", tt.text, o, tt.want)
		}
		if got := dmrd.AppendFreeDMRStatic(nil, []byte(tt.text), enums.Slot1); !slices.Equal(got, tt.ts1) {
			t.Errorf("%q: TS1 %v, want %v", tt.text, got, tt.ts1)
		}
		if got := dmrd.AppendFreeDMRStatic(nil, []byte(tt.text), enums.Slot2); !slices.Equal(got, tt.ts2) {
			t.Errorf("%q: TS2 %v, want %v", tt.text, got, tt.ts2)
		}
	}
}

func TestFreeDMROptionsHarness(t *testing.T) {
	t.Parallel()

	valid := func(tgs []uint32) []uint32 {
		var out []uint32
		for _, tg := range tgs {
			if tg > 9 && (tg < 9990 || tg > 9999) && tg < 16777215 {
				out = append(out, tg)
			}
		}
		return out
	}
	text := []byte("TS1=8,16777215,A93,91;TS2=7,16777216,B94,92;DIAL=0;TIMER=1")
	if got := valid(dmrd.AppendFreeDMRStatic(nil, text, enums.Slot1)); !slices.Equal(got, []uint32{91}) {
		t.Errorf("TS1 %v", got)
	}
	if got := dmrd.AppendFreeDMRStatic(nil, text, enums.Slot2); !slices.Equal(got, []uint32{7, 92}) {
		t.Errorf("TS2 %v", got)
	}
	text = []byte("TS1=91, 92;TS2=93, 94;DIAL=0;TIMER=1")
	if got := dmrd.AppendFreeDMRStatic(nil, text, enums.Slot2); !slices.Equal(got, []uint32{93, 94}) {
		t.Errorf("TS2 %v", got)
	}
	for _, tt := range []struct {
		text string
		want dmrd.FreeDMROptions
	}{
		{"IDENTTG=A;DIAL=91;TIMER=1", dmrd.FreeDMROptions{DialTS2: set(91), Timer: set(1)}},
		{"VOICE=A;TS1=91;DIAL=0;TIMER=1", dmrd.FreeDMROptions{DialTS2: set(0), Timer: set(1)}},
		{"VOICE=2;SINGLE=2;DIAL=0;TIMER=1", dmrd.FreeDMROptions{DialTS2: set(0), Timer: set(1)}},
	} {
		o, err := dmrd.ParseFreeDMROptions([]byte(tt.text))
		if !errors.Is(err, dmrd.ErrText) || !sameOptions(o, tt.want) {
			t.Errorf("%q: %+v, %v", tt.text, o, err)
		}
	}
	o, err := dmrd.ParseFreeDMROptions([]byte("default_dial_ts1=91;TIMER=1"))
	if err != nil || !sameOptions(o, dmrd.FreeDMROptions{DialTS1: set(91), Timer: set(1)}) {
		t.Errorf("default_dial_ts1: %+v, %v", o, err)
	}
	o, err = dmrd.ParseFreeDMROptions([]byte("DIALTG=0;DYNAMIC=0;DIAL=0;TIMER=1"))
	if err != nil || !sameOptions(o, dmrd.FreeDMROptions{DialTS2: set(0), Timer: set(1), DialATG: set(0), Dynamic: set(0)}) {
		t.Errorf("DIALTG: %+v, %v", o, err)
	}
}

func TestFreeDMROptionsPrecedence(t *testing.T) {
	t.Parallel()

	const text = "Type=HBlink;StartRef=4013;DEFAULT_REFLECTOR=9;DIAL=5;RelinkTime=20;TIMER=10;TS1=1,2;TS1_2=262;TS1_1=270;TS1_5=1;" +
		"TS2_STATIC=3;VOICETG=8;OVERRIDE_IDENT_TG=7;UserLink=1;x=1=2;junk;DIAL=6;'VOICE'=\"1\";DIALTO1 = 3 ;\x80TS2Timer=4\x00\x00"
	o, err := dmrd.ParseFreeDMROptions([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	want := dmrd.FreeDMROptions{DialTS2: set(6), Timer: set(20), IdentTG: set(8), Voice: set(1), TS2Timer: set(4)}
	if !sameOptions(o, want) {
		t.Errorf("\n got %+v\nwant %+v", o, want)
	}
	if got := dmrd.AppendFreeDMRStatic(nil, []byte(text), enums.Slot1); !slices.Equal(got, []uint32{270, 262, 1}) {
		t.Errorf("TS1 %v", got)
	}
	if got := dmrd.AppendFreeDMRStatic([]uint32{9}, []byte(text), enums.Slot2); !slices.Equal(got, []uint32{9, 3}) {
		t.Errorf("TS2 %v", got)
	}
	if got := dmrd.AppendFreeDMRStatic(nil, []byte(text), enums.SlotUnknown); got != nil {
		t.Errorf("SlotUnknown %v", got)
	}
	long := bytes.Repeat([]byte{'a'}, dmrd.MaxFreeDMROptionsLen+1)
	if o, err := dmrd.ParseFreeDMROptions(long); !errors.Is(err, dmrd.ErrText) || !sameOptions(o, dmrd.FreeDMROptions{}) || o.Key != nil {
		t.Errorf("long: %+v, %v", o, err)
	}
	if got := dmrd.AppendFreeDMRStatic(nil, append([]byte("TS1=91;"), long...), enums.Slot1); got != nil {
		t.Errorf("long TS1 %v", got)
	}
	if _, err := dmrd.ParseFreeDMROptions([]byte("TIMER=4294967296")); !errors.Is(err, dmrd.ErrText) {
		t.Errorf("overflow: %v", err)
	}
}

// ExampleParseFreeDMROptions reads the options that a hotspot sends to a
// FreeDMR master in an RPTO.
func ExampleParseFreeDMROptions() {
	text := []byte("TS1=91,2350;TS2=;DIAL=4000;VOICE=1;TIMER=10;")
	o, err := dmrd.ParseFreeDMROptions(text)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(dmrd.AppendFreeDMRStatic(nil, text, enums.Slot1), dmrd.AppendFreeDMRStatic(nil, text, enums.Slot2))
	fmt.Println(o.DialTS2.Value, o.Voice.Value, o.Timer.Value, o.Single.State == dmrd.OptionAbsent)
	// Output:
	// [91 2350] []
	// 4000 1 10 true
}

func FuzzFreeDMROptions(f *testing.F) {
	freeDMRSeeds(func(b []byte) { f.Add(b) })
	f.Fuzz(fuzzFreeDMROptions)
}

func freeDMRSeeds(add func([]byte)) {
	add([]byte("KEY=secret;TS1=91,92;TS2=235;DIAL1=4400;DIAL2=2350;TGTO1=10;TGTO2=10;DIALTO1=30;DIALTO2=30;NDD_SLOT=1"))
	add([]byte("Type=HBlink;StartRef=4013;RelinkTime=15;UserLink=1;TS1_1=262;TS1_2=1;TS2_9=5;DIAL=;LANG=en_GB"))
	add([]byte("VOICE=2;SINGLE=A;TS1=8, 1 6,A93;'IDENTTG'=\"9\";DEFAULT_UA_TIMER= 7 \x00"))
}

func fuzzFreeDMROptions(t *testing.T, text []byte) {
	o, err := dmrd.ParseFreeDMROptions(text)
	want, ts1, ts2, wantErr := refFreeDMR(text)
	if wantErr != (err != nil) {
		t.Fatalf("%q: err %v, want error %v", text, err, wantErr)
	}
	key, lang := o.Key, o.Lang
	o.Key, o.Lang = nil, nil
	if !sameOptions(o, want.FreeDMROptions) || dropQuotes(key) != want.key || dropQuotes(lang) != want.lang {
		t.Fatalf("%q:\n got %+v %q %q\nwant %+v %q %q", text, o, key, lang, want.FreeDMROptions, want.key, want.lang)
	}
	if got := dmrd.AppendFreeDMRStatic(nil, text, enums.Slot1); !slices.Equal(got, ts1) {
		t.Fatalf("%q: TS1 %v, want %v", text, got, ts1)
	}
	if got := dmrd.AppendFreeDMRStatic(nil, text, enums.Slot2); !slices.Equal(got, ts2) {
		t.Fatalf("%q: TS2 %v, want %v", text, got, ts2)
	}
}

type refOptions struct {
	dmrd.FreeDMROptions
	key, lang string
}

func dropQuotes(b []byte) string {
	var s []byte
	for _, c := range b {
		if c != '\'' && c != '"' && c < 0x80 {
			s = append(s, c)
		}
	}
	return string(s)
}

func refFreeDMR(text []byte) (o refOptions, ts1, ts2 []uint32, bad bool) {
	if len(text) > dmrd.MaxFreeDMROptionsLen {
		return o, nil, nil, true
	}
	s := dropQuotes(bytes.TrimRight(text, "\x00"))
	opts := map[string]string{}
	for _, x := range strings.Split(s, ";") {
		kv := strings.Split(x, "=")
		if len(kv) == 2 {
			opts[kv[0]] = kv[1]
		}
	}
	move := func(from, to string) {
		if v, ok := opts[from]; ok {
			delete(opts, from)
			opts[to] = v
		}
	}
	moveIfAbsent := func(from, to string) {
		if v, ok := opts[from]; ok {
			delete(opts, from)
			if _, ok := opts[to]; !ok {
				opts[to] = v
			}
		}
	}
	move("TIMER", "DEFAULT_UA_TIMER")
	move("TS1", "TS1_STATIC")
	move("TS2", "TS2_STATIC")
	if _, ok := opts["IDENTTG"]; ok {
		move("IDENTTG", "OVERRIDE_IDENT_TG")
	} else {
		move("VOICETG", "OVERRIDE_IDENT_TG")
	}
	move("IDENT", "VOICE")
	move("DIALTG", "DIAL_A_TG")
	move("DYNAMIC", "DYNAMIC_TG_ROUTING")
	move("DIAL1", "DEFAULT_DIAL_TS1")
	move("DIAL2", "DEFAULT_DIAL_TS2")
	move("TGTO1", "TS1_TG_TIMER")
	move("TGTO2", "TS2_TG_TIMER")
	move("DIALTO1", "TS1_DIAL_TIMER")
	move("DIALTO2", "TS2_DIAL_TIMER")
	move("NDD_SLOT", "NETWORK_DIRECT_DIAL_SLOT")
	move("default_dial_ts1", "DEFAULT_DIAL_TS1")
	move("default_dial_ts2", "DEFAULT_DIAL_TS2")
	moveIfAbsent("DIAL", "DEFAULT_DIAL_TS2")
	moveIfAbsent("DEFAULT_REFLECTOR", "DEFAULT_DIAL_TS2")
	moveIfAbsent("StartRef", "DEFAULT_DIAL_TS2")
	move("RelinkTime", "DEFAULT_UA_TIMER")
	move("TS1Timer", "TS1_TIMER")
	move("TS2Timer", "TS2_TIMER")
	move("TGTimer", "TG_TIMER")
	move("DialTimer", "DIAL_TIMER")
	for _, ts := range []string{"TS1", "TS2"} {
		if v, ok := opts[ts+"_1"]; ok {
			for n := 2; n <= 9; n++ {
				if w, ok := opts[ts+"_"+strconv.Itoa(n)]; ok {
					v += "," + w
				}
			}
			opts[ts+"_STATIC"] = v
		}
	}
	num := func(key string, limit uint64, emptyClears bool) dmrd.OptionValue {
		v, ok := opts[key]
		if !ok {
			return dmrd.OptionValue{}
		}
		v = strings.Trim(v, " \t\n\v\f\r\x1c\x1d\x1e\x1f")
		if v == "" {
			if emptyClears {
				return dmrd.OptionValue{State: dmrd.OptionClear}
			}
			return dmrd.OptionValue{}
		}
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil || n > limit || v[0] == '+' {
			bad = true
			return dmrd.OptionValue{}
		}
		return set(uint32(n))
	}
	const maxV = 1<<32 - 1
	o.DialTS1 = num("DEFAULT_DIAL_TS1", maxV, true)
	o.DialTS2 = num("DEFAULT_DIAL_TS2", maxV, true)
	o.Timer = num("DEFAULT_UA_TIMER", maxV, false)
	o.TS1Timer = num("TS1_TIMER", maxV, false)
	o.TS2Timer = num("TS2_TIMER", maxV, false)
	o.TGTimer = num("TG_TIMER", maxV, false)
	o.DialTimer = num("DIAL_TIMER", maxV, false)
	o.TS1TGTimer = num("TS1_TG_TIMER", maxV, false)
	o.TS2TGTimer = num("TS2_TG_TIMER", maxV, false)
	o.TS1DialTimer = num("TS1_DIAL_TIMER", maxV, false)
	o.TS2DialTimer = num("TS2_DIAL_TIMER", maxV, false)
	o.Voice = num("VOICE", 1, false)
	o.IdentTG = num("OVERRIDE_IDENT_TG", maxV, false)
	o.Single = num("SINGLE", 1, false)
	o.DialATG = num("DIAL_A_TG", 1, false)
	o.Dynamic = num("DYNAMIC_TG_ROUTING", 1, false)
	o.DirectDialSlot = num("NETWORK_DIRECT_DIAL_SLOT", 2, false)
	o.key, o.lang = opts["KEY"], opts["LANG"]
	list := func(v string) []uint32 {
		var out []uint32
		v = strings.Map(func(r rune) rune {
			if strings.ContainsRune(" \t\n\v\f\r\x1c\x1d\x1e\x1f", r) {
				return -1
			}
			return r
		}, v)
		for _, tg := range strings.Split(v, ",") {
			if tg == "" || strings.Trim(tg, "0123456789") != "" {
				continue
			}
			if n, err := strconv.ParseUint(tg, 10, 64); err == nil && n < 1<<24 {
				out = append(out, uint32(n))
			}
		}
		return out
	}
	return o, list(opts["TS1_STATIC"]), list(opts["TS2_STATIC"]), bad
}
