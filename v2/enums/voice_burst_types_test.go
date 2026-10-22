package enums

import "testing"

func TestVoiceBurstTypeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		burstType VoiceBurstType
		expected  string
	}{
		{VoiceBurstUnknown, unknownName},
		{VoiceBurstA, "A"},
		{VoiceBurstB, "B"},
		{VoiceBurstC, "C"},
		{VoiceBurstD, "D"},
		{VoiceBurstE, "E"},
		{VoiceBurstF, "F"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()

			got := tt.burstType.String()
			if got != tt.expected {
				t.Errorf("(%d).String() = %q, want %q", tt.burstType, got, tt.expected)
			}
		})
	}
}

func TestVoiceBurstAtAndIndex(t *testing.T) {
	t.Parallel()

	for i := -1; i <= 6; i++ {
		v, ok := VoiceBurstAt(i)
		if ok != (i >= 0 && i <= 5) {
			t.Fatalf("VoiceBurstAt(%d) ok = %v", i, ok)
		}
		if !ok {
			continue
		}
		if got, ok := v.Index(); !ok || got != i {
			t.Errorf("VoiceBurstAt(%d).Index() = %d, %v", i, got, ok)
		}
	}
	if v, _ := VoiceBurstAt(5); v != VoiceBurstF {
		t.Errorf("VoiceBurstAt(5) = %s, want F", v)
	}
	for _, v := range []VoiceBurstType{VoiceBurstUnknown, VoiceBurstF + 1} {
		if _, ok := v.Index(); ok {
			t.Errorf("(%d).Index() is ok", v)
		}
	}
}

func TestVoiceBurstTypeLCSS(t *testing.T) {
	t.Parallel()

	want := map[VoiceBurstType]LCSS{
		VoiceBurstB: LCSSFirstFragment,
		VoiceBurstC: LCSSContinuationFragment,
		VoiceBurstD: LCSSContinuationFragment,
		VoiceBurstE: LCSSLastFragment,
		VoiceBurstF: LCSSSingleFragment,
	}
	for v := VoiceBurstUnknown; v <= VoiceBurstF+1; v++ {
		got, ok := v.LCSS()
		w, wantOK := want[v]
		if ok != wantOK || got != w {
			t.Errorf("(%s).LCSS() = %s, %v; want %s, %v", v, got, ok, w, wantOK)
		}
	}
}

func TestVoiceBurstTypeString_Invalid(t *testing.T) {
	t.Parallel()

	got := (VoiceBurstType(99)).String()
	if got != "VoiceBurstType(99)" {
		t.Errorf("(99).String() = %q", got)
	}
}
