package trunk_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

const msAddr = 3120001

var (
	idle = [3]byte{}
	busy = [3]byte{0x80, 0x02, 0x02}
)

type msRig struct {
	t   *testing.T
	ms  *trunk.MS
	now time.Duration
	out []trunk.Output
}

func newRig(t *testing.T, cfg *trunk.MSConfig) *msRig {
	t.Helper()
	ms, err := trunk.NewMS(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return &msRig{t: t, ms: ms, out: make([]trunk.Output, 0, trunk.MaxOutputs)}
}

func (r *msRig) feed(raw layer2.RawBurst, cach *[3]byte) []trunk.Output {
	r.now += 60 * time.Millisecond
	r.out = r.ms.Burst(r.now, &raw, layer2.Hint{Slot: enums.Slot1}, cach, r.out[:0])
	return r.out
}

func ack(t testing.TB, r enums.AckReason) layer2.RawBurst {
	t.Helper()
	return bsBurst(t, &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAckOutbound, AckOutboundPDU: &pdu.AckOutboundPDU{
		ReasonCode: byte(r), TargetAddress: msAddr}}, testCC)
}

func sent(t *testing.T, o *trunk.Output) *pdu.CSBK {
	t.Helper()
	b, err := layer2.NewBurstFromBytes(o.Burst)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := b.Data.(*pdu.CSBK)
	if !ok || b.SyncPattern != enums.SyncMSSourcedData || b.SlotType.ColorCode != testCC || c.FEC.Uncorrectable {
		t.Fatalf("OutputBurst: %T, sync %v, cc %d", b.Data, b.SyncPattern, b.SlotType.ColorCode)
	}
	return c
}

func TestMS_RegisterAndCall(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, Seed: 7})
	a := aloha(t)
	wantKinds(t, "C_ALOHA", r.feed(a, &idle), trunk.OutputSite)

	out := r.feed(a, &idle)
	wantKinds(t, "registration C_RAND", out, trunk.OutputBurst)
	c := sent(t, &out[0])
	if c.RandomAccessPDU == nil || c.RandomAccessPDU.ServiceKind != uint8(enums.ServiceRegistration) ||
		c.RandomAccessPDU.TargetAddress != constants.GatewayREGI || c.RandomAccessPDU.SourceAddress != msAddr || c.RandomAccessPDU.ServiceOptions != 1 {
		t.Fatalf("registration C_RAND %+v", c.RandomAccessPDU)
	}
	if out[0].Due != r.now+60*time.Millisecond || out[0].Slot != enums.Slot1 {
		t.Errorf("C_RAND due %v slot %v at %v", out[0].Due, out[0].Slot, r.now)
	}
	if err := r.ms.Call(r.now, true, 9, layer3.VoiceServiceOptions{}); !errors.Is(err, trunk.ErrBusy) {
		t.Errorf("Call while registering = %v", err)
	}
	out = r.feed(ack(t, enums.ReasonRegAccepted), &idle)
	wantKinds(t, "Reg_Accepted", out, trunk.OutputRegistered)
	if !r.ms.Registered() || r.ms.Busy() {
		t.Errorf("Registered %v, Busy %v", r.ms.Registered(), r.ms.Busy())
	}

	if err := r.ms.Call(r.now, true, 9, layer3.VoiceServiceOptions{PriorityLevel: 1}); err != nil {
		t.Fatal(err)
	}
	out = r.feed(a, &idle)
	wantKinds(t, "call C_RAND", out, trunk.OutputBurst)
	if c := sent(t, &out[0]); c.RandomAccessPDU.ServiceKind != uint8(enums.ServiceTalkgroupVoice) || c.RandomAccessPDU.TargetAddress != 9 || c.RandomAccessPDU.ServiceOptions != 1 {
		t.Fatalf("call C_RAND %+v", c.RandomAccessPDU)
	}
	wantKinds(t, "C_WACKD", r.feed(ack(t, enums.ReasonWait), &idle))
	out = r.feed(tvGrant(t, 9, 5, true), &idle)
	wantKinds(t, "TV_GRANT", out, trunk.OutputGrant, trunk.OutputTune)
	if r.ms.State() != trunk.StatePayload || r.ms.Busy() {
		t.Fatalf("State %v, Busy %v", r.ms.State(), r.ms.Busy())
	}

	out = r.ms.EndCall(r.now, r.out[:0])
	wantKinds(t, "EndCall", out, trunk.OutputBurst, trunk.OutputBurst, trunk.OutputBurst, trunk.OutputBurst, trunk.OutputCallEnd, trunk.OutputTune)
	for i := range constants.NMaint {
		c := sent(t, &out[i])
		if c.MaintenancePDU == nil || c.MaintenancePDU.TargetAddress != 9 || c.MaintenancePDU.SourceAddress != msAddr || out[i].Slot != enums.Slot2 ||
			out[i].Due != r.now+time.Duration(i)*60*time.Millisecond {
			t.Errorf("P_MAINT %d: %+v due %v slot %v", i, c.MaintenancePDU, out[i].Due, out[i].Slot)
		}
	}
	if out[4].Reason != trunk.ReasonMaintenance || r.ms.State() != trunk.StateControl {
		t.Errorf("after EndCall: %v, state %v", out[4].Reason, r.ms.State())
	}
}

func TestMS_NoAnswer(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true, RandomTimeout: -1})
	a := aloha(t)
	r.feed(a, &idle)
	if err := r.ms.Call(r.now, false, 3120002, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	wantKinds(t, "withdrawn slot", r.feed(a, &busy))
	wantKinds(t, "no CACH", r.feed(a, nil))
	var sends int
	for range 200 {
		out := r.feed(a, &idle)
		for i := range out {
			switch out[i].Kind {
			case trunk.OutputBurst:
				sends++
				if c := sent(t, &out[i]); c.RandomAccessPDU.ServiceKind != uint8(enums.ServiceIndividualVoice) {
					t.Fatalf("C_RAND %+v", c.RandomAccessPDU)
				}
			case trunk.OutputRefused:
				if out[i].Reason != trunk.ReasonNoAnswer || sends != constants.NRandNR {
					t.Fatalf("refused %v after %d C_RAND, want NoAnswer after %d", out[i].Reason, sends, constants.NRandNR)
				}
				return
			default:
				t.Fatalf("output %v", out[i].Kind)
			}
		}
	}
	t.Fatalf("no OutputRefused after %d C_RAND", sends)
}

func TestMS_Refused(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true})
	a := aloha(t)
	r.feed(a, &idle)
	if err := r.ms.Call(r.now, true, 9, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	wantKinds(t, "C_RAND", r.feed(a, &idle), trunk.OutputBurst)
	out := r.feed(ack(t, enums.ReasonSysBusyRefused), &idle)
	wantKinds(t, "C_NACKD", out, trunk.OutputRefused)
	if out[0].Reason != trunk.ReasonNack || out[0].Ack != enums.ReasonSysBusyRefused || r.ms.Busy() {
		t.Errorf("refused %+v, busy %v", out[0], r.ms.Busy())
	}
}

func TestMS_CallTimeout(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true, CallTimeout: 4 * time.Second, NoSignal: -1})
	a := aloha(t)
	r.feed(a, &idle)
	_ = r.ms.Call(r.now, true, 9, layer3.VoiceServiceOptions{})
	r.feed(a, &idle)
	r.feed(ack(t, enums.ReasonQueuedForResource), &idle)
	at, ok := r.ms.Deadline()
	if !ok || at != r.now+4*time.Second {
		t.Fatalf("Deadline %v, %v, want %v", at, ok, r.now+4*time.Second)
	}
	out := r.ms.Tick(at, r.out[:0])
	wantKinds(t, "TP_Timer", out, trunk.OutputRefused)
	if out[0].Reason != trunk.ReasonTimeout {
		t.Errorf("reason %v", out[0].Reason)
	}
}

func TestMS_Called(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true})
	r.feed(aloha(t), &idle)
	ahoy := bsBurst(t, &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAhoy, AhoyPDU: &pdu.AhoyPDU{
		ServiceKind: uint8(enums.ServiceIndividualVoice), TargetAddress: msAddr, SourceAddress: 3120002}}, testCC)
	out := r.feed(ahoy, &busy)
	wantKinds(t, "C_AHOY", out, trunk.OutputAhoy, trunk.OutputBurst)
	if out[0].Grant.Source != 3120002 {
		t.Errorf("Ahoy %+v", out[0].Grant)
	}
	c := sent(t, &out[1])
	if c.AckInboundPDU == nil || enums.AckReason(c.AckInboundPDU.ReasonCode) != enums.ReasonMSAccepted || c.AckInboundPDU.TargetAddress != 3120002 {
		t.Fatalf("C_ACKU %+v", c.AckInboundPDU)
	}
	pv := bsBurst(t, &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKPrivateVoiceGrant, PrivateVoiceGrantPDU: &pdu.PrivateVoiceGrantPDU{
		PhysicalChannel: 8, TargetAddress: msAddr, SourceAddress: 3120002}}, testCC)
	wantKinds(t, "PV_GRANT", r.feed(pv, &idle), trunk.OutputGrant, trunk.OutputTune)
	other := bsBurst(t, &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKPrivateVoiceGrant, PrivateVoiceGrantPDU: &pdu.PrivateVoiceGrantPDU{
		PhysicalChannel: 9, TargetAddress: 3120005, SourceAddress: 3120002}}, testCC)
	r2 := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true})
	r2.feed(aloha(t), &idle)
	wantKinds(t, "PV_GRANT for another MS", r2.feed(other, &idle), trunk.OutputGrant)
}

func TestNewMS_Errors(t *testing.T) {
	t.Parallel()
	var ce *dmrgo.ConfigError
	for _, cfg := range []*trunk.MSConfig{nil, {}, {Address: 0xFFFFFF}, {Address: 1, HuntLen: 65}} {
		if _, err := trunk.NewMS(cfg); !errors.As(err, &ce) {
			t.Errorf("NewMS(%+v) = %v, want a *dmrgo.ConfigError", cfg, err)
		}
	}
	r := newRig(t, &trunk.MSConfig{Address: msAddr})
	if err := r.ms.Call(0, true, 9, layer3.VoiceServiceOptions{}); !errors.Is(err, trunk.ErrNotOnControl) {
		t.Errorf("Call before a TSCC = %v", err)
	}
}
