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
