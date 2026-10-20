package rf

import (
	"errors"
	"testing"
)

func TestWriteTooLong(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 2)
	x := noisyBaseband(slots, 900, 25, 3)
	const most = 1 << 13
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		cfg := Config{Rate: 5 * basebandRate, Tier: tier, MaxWrite: most}
		whole := mustNew(cfg)
		if b, err := whole.WriteBaseband(x); b != nil || !errors.Is(err, ErrWriteTooLong) {
			t.Fatalf("tier %d: a WriteBaseband of %d samples gave %d bursts and %v", tier, len(x), len(b), err)
		}
		a, b := mustNew(cfg), mustNew(cfg)
		var got, want []Burst
		for off := 0; off < len(x); off += most / 2 {
			end := min(off+most/2, len(x))
			if end < len(x) {
				if out, err := a.WriteBaseband(x[off : end+1]); out != nil || !errors.Is(err, ErrWriteTooLong) {
					t.Fatalf("tier %d: a WriteBaseband of %d samples gave %d bursts and %v", tier, end+1-off, len(out), err)
				}
			}
			got = append(got, must(a.WriteBaseband(x[off:end]))...)
			want = append(want, must(b.WriteBaseband(x[off:end]))...)
		}
		got, want = append(got, a.Flush()...), append(want, b.Flush()...)
		if len(want) == 0 {
			t.Fatalf("tier %d: no bursts", tier)
		}
		sameBursts(t, "after ErrWriteTooLong", want, got)
		cfg.OnBurst = func(*Burst) {}
		if _, err := mustNew(cfg).WriteBaseband(x); err != nil {
			t.Fatalf("tier %d: with OnBurst: %v", tier, err)
		}
	}
}

func TestDiscWriteTooLong(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 2)
	audio := discAudio(noisyBaseband(slots, 900, 25, 3), benchAudioHz)
	const most = 1 << 10
	a, err := NewDisc(DiscConfig{Hz: benchAudioHz, MaxWrite: most})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewDisc(DiscConfig{Hz: benchAudioHz, MaxWrite: most})
	if err != nil {
		t.Fatal(err)
	}
	var got, want []Burst
	for off := 0; off < len(audio); off += most {
		end := min(off+most, len(audio))
		if len(audio)-off > 2*most {
			if out, err := a.Write(audio[off:]); out != nil || !errors.Is(err, ErrWriteTooLong) {
				t.Fatalf("a Write of %d samples gave %d bursts and %v", len(audio)-off, len(out), err)
			}
		}
		got = append(got, must(a.Write(audio[off:end]))...)
		want = append(want, must(b.Write(audio[off:end]))...)
	}
	got, want = append(got, a.Flush()...), append(want, b.Flush()...)
	if len(want) == 0 {
		t.Fatal("no bursts")
	}
	sameBursts(t, "after ErrWriteTooLong", want, got)
	var r DiscReceiver
	if err := r.Init(DiscConfig{Hz: benchAudioHz}); err != nil {
		t.Fatal(err)
	}
	if out, err := r.Write(audio[:1]); out != nil || !errors.Is(err, ErrWriteTooLong) {
		t.Fatalf("Init without OnBurst: Write gave %d bursts and %v", len(out), err)
	}
	n := len(r.AppendBursts(nil, audio))
	if f := r.Flush(); f != nil {
		t.Fatalf("Init without OnBurst: Flush gave %d bursts", len(f))
	}
	if n += len(r.AppendFlush(nil)); n != len(want) {
		t.Fatalf("Init without OnBurst: AppendBursts and Flush gave %d bursts, NewDisc %d", n, len(want))
	}
}
