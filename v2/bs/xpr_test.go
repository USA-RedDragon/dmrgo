package bs_test

import (
	"bufio"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type xprBurst struct {
	at    time.Duration
	cach  layer2.CACH
	burst layer2.RawBurst
}

func loadXPR(t *testing.T) []xprBurst {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out []xprBurst
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		fs := strings.Fields(line)
		sample, err := strconv.Atoi(fs[0])
		if err != nil {
			t.Fatal(err)
		}
		cach, err := hex.DecodeString(fs[1])
		if err != nil {
			t.Fatal(err)
		}
		raw, err := hex.DecodeString(fs[3])
		if err != nil {
			t.Fatal(err)
		}
		b := xprBurst{at: time.Duration(sample) * time.Second / 48000, cach: layer2.DecodeCACHField([3]byte(cach))}
		copy(b.burst[:], raw)
		out = append(out, b)
	}
	return out
}

func slotOf(c layer2.CACH) enums.Slot {
	if c.TACT.TDMAChannel {
		return enums.Slot2
	}
	return enums.Slot1
}

func dataType(b *layer2.RawBurst) (enums.DataType, bool) {
	p, _ := b.Sync()
	if p != enums.SyncBSSourcedData && p != enums.SyncMSSourcedData {
		return 0, false
	}
	return b.SlotType().DataType, true
}

func runXPR(t *testing.T, xpr []xprBurst, cfg *bs.Config) []bs.Output {
	t.Helper()
	r, err := bs.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]bs.Output, 0, bs.MaxOutputs)
	var got []bs.Output
	keep := func() {
		for _, o := range out {
			if o.Kind == bs.OutputBurst {
				got = append(got, o)
			}
		}
	}
	end := xpr[len(xpr)-1].at + 6*time.Second
	now := xpr[0].at
	i := 0
	for now < end {
		for ; i < len(xpr) && xpr[i].at <= now; i++ {
			x := &xpr[i]
			if slotOf(x.cach) != enums.Slot2 {
				continue
			}
			if dt, ok := dataType(&x.burst); ok && dt == enums.DataTypeIdle {
				continue
			}
			out, err = r.Inbound(x.at, &x.burst, layer2.Hint{Slot: enums.Slot2}, bs.Source{Peer: 1}, out[:0])
			if err != nil {
				t.Fatal(err)
			}
			keep()
		}
		out = r.Tick(now, out[:0])
		keep()
		now += bs.SlotTime
	}
	return got
}

type xprRef struct {
	idle, header, terminator layer2.RawBurst
	voice                    [][layer2.VoiceSlotsPerBurst][9]byte
}

func refOf(xpr []xprBurst) xprRef {
	var ref xprRef
	for i := range xpr {
		x := &xpr[i]
		dt, isData := dataType(&x.burst)
		switch {
		case slotOf(x.cach) == enums.Slot1 && isData && dt == enums.DataTypeIdle:
			ref.idle = x.burst
		case isData && dt == enums.DataTypeVoiceLCHeader:
			ref.header = x.burst
		case isData && dt == enums.DataTypeTerminatorWithLC:
			ref.terminator = x.burst
		case !isData:
			ref.voice = append(ref.voice, x.burst.VoiceSlots())
		}
	}
	return ref
}

type xprCheck struct {
	ref                                xprRef
	asm                                layer2.ShortLCAssembler
	voice                              [][layer2.VoiceSlotsPerBurst][9]byte
	header, terminator, idle, activity bool
}

func (c *xprCheck) burst(t *testing.T, i int, o *bs.Output) {
	t.Helper()
	if !o.HasCACH || o.To != bs.ToRF {
		t.Fatalf("burst %d: To %v HasCACH %v", i, o.To, o.HasCACH)
	}
	cach := layer2.DecodeCACHField(o.CACH)
	if slotOf(cach) != o.Slot {
		t.Fatalf("burst %d: TC gives %v, slot %v", i, slotOf(cach), o.Slot)
	}
	dt, isData := dataType(&o.Burst)
	if o.Slot == enums.Slot1 && o.Burst != c.ref.idle {
		t.Fatalf("burst %d: TS1 is not the XPR Idle burst", i)
	}
	if o.Slot == enums.Slot2 {
		c.slot2(t, i, o, dt, isData)
	}
	busy := o.Slot == enums.Slot1 && c.header && !c.idle
	if o.Slot == enums.Slot2 && cach.TACT.AccessType || busy && !cach.TACT.AccessType {
		t.Errorf("burst %d: AT %v before %v", i, cach.TACT.AccessType, o.Slot)
	}
	slc, done, err := c.asm.Add(cach)
	if err != nil || !done || slc.ActivityUpdate == nil {
		return
	}
	c.activity = true
	if au := slc.ActivityUpdate; au.TS2ActivityID != enums.ActivityGroupVoice || au.HashTS2 != 63 || au.TS1ActivityID != enums.ActivityNoActivity {
		t.Errorf("burst %d: Activity Update %v, want the XPR one", i, au)
	}
}

func (c *xprCheck) slot2(t *testing.T, i int, o *bs.Output, dt enums.DataType, isData bool) {
	t.Helper()
	switch {
	case !isData:
		c.voice = append(c.voice, o.Burst.VoiceSlots())
	case dt == enums.DataTypeVoiceLCHeader:
		c.header = true
		if o.Burst != c.ref.header {
			t.Errorf("burst %d: Voice LC Header differs from the XPR one", i)
		}
	case dt == enums.DataTypeTerminatorWithLC:
		c.terminator = true
		if o.Burst != c.ref.terminator {
			t.Errorf("burst %d: Terminator differs from the XPR one", i)
		}
	case dt == enums.DataTypeIdle:
		c.idle = c.idle || c.terminator
	}
}

func TestXPR8400Outbound(t *testing.T) {
	t.Parallel()
	xpr := loadXPR(t)
	got := runXPR(t, xpr, &bs.Config{ColorCode: 7, CACH: true})
	c := xprCheck{ref: refOf(xpr)}
	for i := range got {
		if i > 0 && got[i-1].Slot == got[i].Slot {
			t.Fatalf("burst %d: slot %v twice", i, got[i].Slot)
		}
		c.burst(t, i, &got[i])
	}
	if !c.header || !c.terminator || !c.idle || !c.activity {
		t.Fatalf("header %v, terminator %v, idle after hang %v, activity %v", c.header, c.terminator, c.idle, c.activity)
	}
	if len(c.voice) != len(c.ref.voice) {
		t.Fatalf("%d voice bursts, XPR sent %d", len(c.voice), len(c.ref.voice))
	}
	for i := range c.voice {
		if c.voice[i] != c.ref.voice[i] {
			t.Errorf("voice burst %d differs from the XPR one", i)
		}
	}
}
