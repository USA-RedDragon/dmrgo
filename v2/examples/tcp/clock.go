package main

import (
	"container/heap"
	"sync"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip"
)

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) //nolint:gochecknoglobals // fixed start of the simulated clock

type simTimer struct {
	c     *simClock
	f     func()
	at    time.Duration
	index int
}

func (t *simTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	if t.index < 0 {
		return false
	}
	heap.Remove(&t.c.timers, t.index)
	return true
}

func (t *simTimer) Reset(d time.Duration) {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	if t.index >= 0 {
		heap.Remove(&t.c.timers, t.index)
	}
	t.at = t.c.now + max(d, 0)
	heap.Push(&t.c.timers, t)
}

type timerHeap []*simTimer

func (h timerHeap) Len() int           { return len(h) }
func (h timerHeap) Less(i, j int) bool { return h[i].at < h[j].at }
func (h timerHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index, h[j].index = i, j
}

func (h *timerHeap) Push(x any) {
	t, _ := x.(*simTimer)
	t.index = len(*h)
	*h = append(*h, t)
}

func (h *timerHeap) Pop() any {
	old := *h
	t := old[len(old)-1]
	*h = old[:len(old)-1]
	t.index = -1
	return t
}

type simClock struct {
	mu     sync.Mutex
	now    time.Duration
	timers timerHeap
}

func (c *simClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return epoch.Add(c.now)
}

func (c *simClock) NowMonotonic() tcpip.MonotonicTime {
	c.mu.Lock()
	defer c.mu.Unlock()
	return tcpip.MonotonicTime{}.Add(c.now)
}

func (c *simClock) AfterFunc(d time.Duration, f func()) tcpip.Timer {
	t := &simTimer{c: c, f: f, index: -1}
	t.Reset(d)
	return t
}

func (c *simClock) next() (time.Duration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.timers) == 0 {
		return 0, false
	}
	return c.timers[0].at, true
}

func (c *simClock) advance(to time.Duration) int {
	n := 0
	for {
		c.mu.Lock()
		if len(c.timers) == 0 || c.timers[0].at > to {
			c.now = max(c.now, to)
			c.mu.Unlock()
			return n
		}
		t, _ := heap.Pop(&c.timers).(*simTimer)
		c.now = max(c.now, t.at)
		c.mu.Unlock()
		t.f()
		n++
	}
}
