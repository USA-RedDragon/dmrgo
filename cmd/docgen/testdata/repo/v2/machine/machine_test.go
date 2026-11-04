package machine

import "testing"

func TestLamp(t *testing.T) {
	l, err := NewLamp(Config{})
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Start(0)
	l.Tick(DefaultWait)
	_ = l.Start(0)
	_ = l.Stop()
	l.Break()
}

func TestCounter(t *testing.T) {
	var c Counter
	c.Add()
	c.Add()
	c.Clear()
}

func TestPair(t *testing.T) {
	var p Pair
	p.Raise(SideB)
	_ = hold
}

func TestAllocs(t *testing.T) {
	l, err := NewLamp(Config{})
	if err != nil {
		t.Fatal(err)
	}
	var c Counter
	var p Pair
	n := testing.AllocsPerRun(10, func() {
		_ = l.Start(0)
		l.Tick(DefaultWait)
		_ = l.Stop()
		c.Add()
		c.Clear()
		p.Raise(SideB)
	})
	if n != 0 {
		t.Errorf("%v allocations per run", n)
	}
}
