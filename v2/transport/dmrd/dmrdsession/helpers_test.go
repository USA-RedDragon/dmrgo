package dmrdsession_test

import (
	"bufio"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

type capLine struct {
	at  time.Duration
	dir string
	p   []byte
}

func readCapture(t testing.TB, name string) []capLine {
	t.Helper()
	f, err := os.Open("../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []capLine
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 1<<16)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		var l capLine
		switch len(fs) {
		case 4:
			l.at = clock(t, fs[0])
			l.dir = fs[1]
			fs = fs[3:]
		case 2:
			l.dir = fs[0]
			fs = fs[1:]
		default:
			t.Fatalf("line %q", sc.Text())
		}
		p, err := hex.DecodeString(fs[0])
		if err != nil {
			t.Fatal(err)
		}
		l.p = p
		out = append(out, l)
	}
	return out
}

func clock(t testing.TB, s string) time.Duration {
	t.Helper()
	hms := strings.Split(s, ":")
	if len(hms) != 3 {
		t.Fatalf("time %q", s)
	}
	h, _ := strconv.Atoi(hms[0])
	m, _ := strconv.Atoi(hms[1])
	sec, err := strconv.ParseFloat(hms[2], 64)
	if err != nil {
		t.Fatal(err)
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec*float64(time.Second))
}

func kinds(out []dmrdsession.Output) []dmrdsession.OutputKind {
	k := make([]dmrdsession.OutputKind, len(out))
	for i, o := range out {
		k[i] = o.Kind
	}
	return k
}

func sends(out []dmrdsession.Output) [][]byte {
	var s [][]byte
	for _, o := range out {
		if o.Kind == dmrdsession.OutputSend {
			s = append(s, append([]byte(nil), o.Packet...))
		}
	}
	return s
}

func find(out []dmrdsession.Output, k dmrdsession.OutputKind) (dmrdsession.Output, bool) {
	for _, o := range out {
		if o.Kind == k {
			return o, true
		}
	}
	return dmrdsession.Output{}, false
}

func command(t testing.TB, p []byte, d dmrd.Dialect) dmrd.Command {
	t.Helper()
	return dmrd.Classify(p, d)
}

func mmdvmConfig(id uint32) []byte {
	var c dmrd.Config
	copy(c[:], "RPTC")
	for i := 4; i < len(c); i++ {
		c[i] = ' '
	}
	c.SetID(id)
	_ = c.SetCallsign([]byte("N0CALL"))
	_ = c.SetRXFrequency(444000000)
	_ = c.SetTXFrequency(449000000)
	_ = c.SetColorCode(1)
	_ = c.SetSlots(3)
	return c[:]
}

func hbrpConfig(id uint32) []byte {
	var c dmrd.HBRPConfig
	copy(c[:], "RPTC")
	for i := 4; i < len(c); i++ {
		c[i] = ' '
	}
	c.SetID(id)
	_ = c.SetCallsign([]byte("DL5DI"))
	_ = c.SetRXFrequency(434787500)
	_ = c.SetTXFrequency(434787500)
	_ = c.SetColorCode(1)
	return c[:]
}

type link struct {
	t       *testing.T
	peer    *dmrdsession.Peer
	master  *dmrdsession.Master
	now     time.Duration
	salt    uint32
	pass    []byte
	events  []dmrdsession.Output
	drop    bool
	toPeer  [][]byte
	toMastr [][]byte
}

func (l *link) fromPeer(out []dmrdsession.Output) {
	for _, o := range out {
		if o.Kind == dmrdsession.OutputSend {
			if !l.drop {
				l.toMastr = append(l.toMastr, append([]byte(nil), o.Packet...))
			}
			continue
		}
		l.events = append(l.events, o)
	}
}

func (l *link) fromMaster(out []dmrdsession.Output) {
	for _, o := range out {
		switch o.Kind {
		case dmrdsession.OutputSend:
			if !l.drop {
				l.toPeer = append(l.toPeer, append([]byte(nil), o.Packet...))
			}
			continue
		case dmrdsession.OutputLogin:
			l.events = append(l.events, o)
			l.fromMaster(l.master.Admit(l.salt, l.pass, nil))
			continue
		default:
		}
		l.events = append(l.events, o)
	}
}

func (l *link) pump() {
	for range 100 {
		if len(l.toPeer) == 0 && len(l.toMastr) == 0 {
			return
		}
		for len(l.toMastr) > 0 {
			p := l.toMastr[0]
			l.toMastr = l.toMastr[1:]
			l.fromMaster(l.master.Packet(l.now, p, nil))
		}
		for len(l.toPeer) > 0 {
			p := l.toPeer[0]
			l.toPeer = l.toPeer[1:]
			l.fromPeer(l.peer.Packet(l.now, p, nil))
		}
	}
	l.t.Fatal("link does not settle")
}

func (l *link) advance(to time.Duration) {
	for range 100000 {
		at, ok := l.peer.Deadline()
		mat, mok := l.master.Deadline()
		next := to
		if ok && at < next {
			next = at
		}
		if mok && mat < next {
			next = mat
		}
		if next < l.now {
			next = l.now
		}
		l.now = next
		l.fromPeer(l.peer.Tick(l.now, nil))
		l.fromMaster(l.master.Tick(l.now, nil))
		l.pump()
		if next == to {
			return
		}
	}
	l.t.Fatal("timers do not advance")
}
