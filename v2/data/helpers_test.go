package data_test

import (
	"bufio"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

type timedBurst struct {
	at  time.Duration
	raw layer2.RawBurst
}

func readBin(t *testing.T, path string, start time.Duration) []timedBurst {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []timedBurst
	for off := 0; off+33 <= len(b); off += 33 {
		out = append(out, timedBurst{at: start + time.Duration(off/33)*60*time.Millisecond, raw: layer2.RawBurst(b[off : off+33])})
	}
	return out
}

func parseTimed(path string) ([]timedBurst, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []timedBurst
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, errors.New("bad line " + line)
		}
		ms, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return nil, err
		}
		raw, err := hex.DecodeString(fields[1])
		if err != nil || len(raw) != 33 {
			return nil, errors.New("bad burst " + fields[1])
		}
		out = append(out, timedBurst{at: time.Duration(ms * float64(time.Millisecond)), raw: layer2.RawBurst(raw)})
	}
	return out, s.Err()
}

func readTimed(t *testing.T, path string) []timedBurst {
	t.Helper()
	b, err := parseTimed(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func readTimedExample(path string) []timedBurst {
	b, _ := parseTimed(path)
	return b
}

func newReceiver(t testing.TB, cfg *data.ReceiverConfig) *data.Receiver {
	t.Helper()
	r, err := data.NewReceiver(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func replay(t testing.TB, r *data.Receiver, bursts []timedBurst) []data.Event {
	t.Helper()
	var all []data.Event
	ev := make([]data.Event, 0, data.MaxEvents)
	keep := func(ev []data.Event) {
		if len(ev) > data.MaxEvents {
			t.Fatalf("%d events, more than MaxEvents", len(ev))
		}
		if n := payloadOctets(ev); n > r.MaxOutputOctets() {
			t.Fatalf("payloads of %d octets, more than MaxOutputOctets %d", n, r.MaxOutputOctets())
		}
		for _, e := range ev {
			e.Message.Payload = append([]byte(nil), e.Message.Payload...)
			all = append(all, e)
		}
	}
	for i := range bursts {
		if at, ok := r.Deadline(); ok && at <= bursts[i].at {
			keep(r.Tick(bursts[i].at, ev[:0]))
		}
		keep(r.Burst(bursts[i].at, &bursts[i].raw, layer2.Hint{}, ev[:0]))
	}
	for {
		at, ok := r.Deadline()
		if !ok {
			break
		}
		keep(r.Tick(at, ev[:0]))
	}
	return all
}

func kinds(ev []data.Event) string {
	var b strings.Builder
	for i, e := range ev {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(e.Kind.String())
		switch e.Kind {
		case data.EventRespond, data.EventResponse:
			b.WriteString("(" + e.Response.String() + ")")
		case data.EventCallEnd:
			b.WriteString("(" + e.Reason.String() + ")")
		case data.EventCallStart, data.EventDelivered, data.EventReassembled:
		}
	}
	return b.String()
}

func setHeader[P any](t testing.TB, set func(*pdu.DataHeader, P) error, p P) pdu.DataHeader {
	t.Helper()
	var h pdu.DataHeader
	if err := set(&h, p); err != nil {
		t.Fatal(err)
	}
	return h
}

func confirmedOf(t testing.TB, h *pdu.DataHeader) pdu.ConfirmedDataHeader {
	t.Helper()
	c, ok := h.AsConfirmed()
	if !ok {
		t.Fatalf("header %v is not a C_HEAD", h)
	}
	return c
}

func editConfirmed(t testing.TB, h *pdu.DataHeader, edit func(*pdu.ConfirmedDataHeader)) {
	t.Helper()
	c := confirmedOf(t, h)
	edit(&c)
	if err := h.SetConfirmed(c); err != nil {
		t.Fatal(err)
	}
}
