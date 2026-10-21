package trunk_test

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

type airBurst struct {
	due  time.Duration
	ch   uint16
	slot enums.Slot
	raw  layer2.RawBurst
}

type serviceSite struct {
	tscc      *trunk.TSCC
	ms        []*trunk.MS
	ch        []uint16
	now       time.Duration
	air       []airBurst
	tout      []trunk.TSCCOutput
	mout      []trunk.Output
	grantSlot enums.Slot
	onMS      func(i int, o *trunk.Output)
	onTSCC    func(o *trunk.TSCCOutput)
}

func newServiceSite(cfg *trunk.TSCCConfig, ms ...*trunk.MSConfig) *serviceSite {
	sys := pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 12, Site: 3, Par: 2}
	cfg.SysCode, cfg.ColorCode, cfg.Channel, cfg.Payload, cfg.PayloadLen, cfg.Reg = sys, 3, 1, [trunk.MaxPayloadChannels]uint16{10}, 1, true
	ts, _ := trunk.NewTSCC(cfg)
	s := &serviceSite{tscc: ts, tout: make([]trunk.TSCCOutput, 0, trunk.MaxTSCCOutputs), mout: make([]trunk.Output, 0, trunk.MaxOutputs)}
	for _, c := range ms {
		m, _ := trunk.NewMS(c)
		s.ms, s.ch = append(s.ms, m), append(s.ch, 1)
	}
	return s
}

func (s *serviceSite) take(i int, out []trunk.Output) {
	for k := range out {
		o := &out[k]
		switch o.Kind {
		case trunk.OutputTune:
			s.ch[i] = max(o.Tune.Channel, 1)
			if o.Tune.Slot != enums.SlotUnknown {
				s.grantSlot = o.Tune.Slot
			}
		case trunk.OutputBurst:
			s.air = append(s.air, airBurst{due: o.Due, ch: s.ch[i], slot: o.Slot, raw: o.Burst})
		default:
		}
		if s.onMS != nil {
			s.onMS(i, o)
		}
	}
}

func (s *serviceSite) tsccOut() {
	for k := range s.tout {
		if s.onTSCC != nil && s.tout[k].Kind != trunk.TSCCBurst {
			s.onTSCC(&s.tout[k])
		}
	}
}

func (s *serviceSite) run(d time.Duration) {
	for end := s.now + d; s.now < end; s.now += 30 * time.Millisecond {
		keep := s.air[:0]
		for _, b := range s.air {
			if b.due > s.now {
				keep = append(keep, b)
				continue
			}
			s.tout = s.tscc.Inbound(s.now, b.ch, b.slot, &b.raw, layer2.Hint{}, s.tout[:0])
			s.tsccOut()
		}
		s.air = keep
		s.tout = s.tscc.Tick(s.now, s.tout[:0])
		bursts := append([]trunk.TSCCOutput(nil), s.tout...)
		s.tsccOut()
		for i, m := range s.ms {
			s.mout = m.Tick(s.now, s.mout[:0])
			s.take(i, s.mout)
		}
		for _, b := range bursts {
			if b.Kind != trunk.TSCCBurst {
				continue
			}
			for i, m := range s.ms {
				if s.ch[i] == b.Channel {
					s.mout = m.Burst(s.now, &b.Burst, layer2.Hint{Slot: b.Slot}, &b.CACH, s.mout[:0])
					s.take(i, s.mout)
				}
			}
		}
	}
}

// ExampleMS_SendStatus sends a status call from one MS to another through
// a TSCC, and polls the status of the other MS.
func ExampleMS_SendStatus() {
	s := newServiceSite(&trunk.TSCCConfig{}, &trunk.MSConfig{Address: 3120001, Seed: 1}, &trunk.MSConfig{Address: 3120002, Seed: 2})
	s.run(3 * time.Second)
	s.onMS = func(i int, o *trunk.Output) {
		switch o.Kind {
		case trunk.OutputStatus:
			fmt.Println("MS", i, "status", o.Status, "from", o.Grant.Source)
		case trunk.OutputAccepted:
			fmt.Println("MS", i, "delivered")
		default:
		}
	}
	_ = s.ms[0].SendStatus(s.now, false, 3120002, 42)
	s.run(2 * time.Second)
	_ = s.ms[1].SetStatus(7)
	_ = s.ms[0].PollStatus(s.now, 3120002)
	s.run(2 * time.Second)
	// Output:
	// MS 1 status 42 from 3120001
	// MS 0 delivered
	// MS 1 status 127 from 3120001
	// MS 0 status 7 from 3120002
}

// ExampleTSCC_Stun stuns an MS, which then refuses calls, and revives it.
func ExampleTSCC_Stun() {
	s := newServiceSite(&trunk.TSCCConfig{}, &trunk.MSConfig{Address: 3120001, Seed: 1})
	s.run(3 * time.Second)
	s.onTSCC = func(o *trunk.TSCCOutput) {
		if o.Kind == trunk.TSCCStun {
			fmt.Println(o.Stun, o.Address, o.Ack)
		}
	}
	_ = s.tscc.Stun(s.now, 3120001, false)
	s.run(time.Second)
	fmt.Println(s.ms[0].Call(s.now, true, 9, layer3.VoiceServiceOptions{}))
	_ = s.tscc.Stun(s.now, 3120001, true)
	s.run(time.Second)
	fmt.Println("stunned", s.ms[0].Stunned())
	// Output:
	// Stun 3120001 MS_Accepted
	// trunk: stunned
	// Revive 3120001 MS_Accepted
	// stunned false
}

// ExampleMS_Interrupt interrupts the MS that talks on a talkgroup call: the
// TSCC accepts the request and sends the cease command on the reverse
// channel.
func ExampleMS_Interrupt() {
	tg := [trunk.MaxTalkgroups]uint32{9}
	s := newServiceSite(&trunk.TSCCConfig{TransmitInterrupt: true},
		&trunk.MSConfig{Address: 3120001, Talkgroups: tg, TalkgroupsLen: 1, Seed: 1},
		&trunk.MSConfig{Address: 3120002, Talkgroups: tg, TalkgroupsLen: 1, Seed: 2})
	s.run(3 * time.Second)
	_ = s.ms[0].Call(s.now, true, 9, layer3.VoiceServiceOptions{})
	s.run(time.Second)
	ceased := false
	s.onMS = func(i int, o *trunk.Output) {
		switch {
		case o.Kind == trunk.OutputAccepted:
			fmt.Println("MS", i, "interrupt accepted")
		case o.Kind == trunk.OutputReverseChannel && i == 0 && !ceased:
			fmt.Println("MS", i, "got", o.RC)
			ceased = true
		default:
		}
	}
	s.onTSCC = func(o *trunk.TSCCOutput) {
		if o.Kind == trunk.TSCCInterrupt {
			fmt.Println("transmission stopped for", o.Address)
		}
	}
	slot := s.grantSlot
	s.mout, _ = s.ms[1].Interrupt(s.now, false, s.mout[:0])
	s.take(1, s.mout)
	var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	for i := 0; i < 60 && !ceased; i++ {
		b, _ := layer2.BuildVoiceBurst(frames, enums.VoiceBurstA+enums.VoiceBurstType(i%6), pdu.EmbeddedSignalling{ColorCode: 3}, [4]byte{}, layer2.SyncSourceMS)
		s.air = append(s.air, airBurst{due: s.now, ch: s.ch[0], slot: slot, raw: b})
		s.run(60 * time.Millisecond)
	}
	s.run(time.Second)
	// Output:
	// MS 1 interrupt accepted
	// MS 0 got Cease Transmission Command
	// transmission stopped for 3120002
}

// ExampleTSCC_MassRegister asks every MS on the TSCC to register again
// within one second.
func ExampleTSCC_MassRegister() {
	s := newServiceSite(&trunk.TSCCConfig{}, &trunk.MSConfig{Address: 3120001, Seed: 1}, &trunk.MSConfig{Address: 3120002, Seed: 2})
	s.run(3 * time.Second)
	s.onMS = func(i int, o *trunk.Output) {
		if o.Kind == trunk.OutputRegistered {
			fmt.Println("MS", i, "registered again")
		}
	}
	_ = s.tscc.MassRegister(2, 0, 0)
	s.run(2 * time.Second)
	// Unordered output:
	// MS 0 registered again
	// MS 1 registered again
}

// ExampleMS_AdviseIP gives the TSCC the IPv4 address of an MS.
func ExampleMS_AdviseIP() {
	s := newServiceSite(&trunk.TSCCConfig{IP: true}, &trunk.MSConfig{Address: 3120001, Seed: 1})
	s.run(3 * time.Second)
	s.onTSCC = func(o *trunk.TSCCOutput) {
		if o.Kind == trunk.TSCCIPAdvice {
			fmt.Println("MS", o.Address, "has", o.IP[0], o.IP[1], o.IP[2], o.IP[3])
		}
	}
	s.onMS = func(_ int, o *trunk.Output) {
		if o.Kind == trunk.OutputAccepted {
			fmt.Println("accepted", o.Ack)
		}
	}
	_ = s.ms[0].AdviseIP(s.now, [16]byte{192, 0, 2, 7}, false, false)
	s.run(2 * time.Second)
	// Output:
	// MS 3120001 has 192 0 2 7
	// accepted Reg_Accepted
}

// ExampleMS_SetSupplementary sends supplementary_user data with an
// individual voice call. The called MS gets it as a UDT before the grant.
func ExampleMS_SetSupplementary() {
	s := newServiceSite(&trunk.TSCCConfig{Supplementary: true}, &trunk.MSConfig{Address: 3120001, Seed: 1}, &trunk.MSConfig{Address: 3120002, Seed: 2})
	s.run(3 * time.Second)
	s.onMS = func(i int, o *trunk.Output) {
		switch {
		case i == 1 && o.Kind == trunk.OutputUDT:
			u := s.ms[1].UDT()
			fmt.Printf("MS 1 got %q\n", u.Data[:9])
		case i == 1 && o.Kind == trunk.OutputGrant:
			fmt.Println("MS 1 got the grant from", o.Grant.Source)
		default:
		}
	}
	_ = s.ms[0].SetSupplementary(3, []byte("room 4711"))
	_ = s.ms[0].Call(s.now, false, 3120002, layer3.VoiceServiceOptions{IsSuppData: true})
	s.run(3 * time.Second)
	// Output:
	// MS 1 got "room 4711"
	// MS 1 got the grant from 3120001
}

// ExampleMS_Asleep registers an MS with power save: it sleeps between the
// power save frames that the TSCC gave it.
func ExampleMS_Asleep() {
	s := newServiceSite(&trunk.TSCCConfig{PowerSave: true}, &trunk.MSConfig{Address: 3120001, Seed: 1, PowerSave: 2})
	s.run(3 * time.Second)
	fmt.Println("offset", s.ms[0].PowerSaveOffset())
	sleeps := 0
	s.onMS = func(_ int, o *trunk.Output) {
		if o.Kind == trunk.OutputSleep {
			sleeps++
		}
	}
	s.run(10 * time.Second)
	fmt.Println("slept", sleeps > 3, "state", s.ms[0].State())
	// Output:
	// offset 2
	// slept true state Control
}
