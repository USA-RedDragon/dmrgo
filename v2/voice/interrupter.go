package voice

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// Limits of InterrupterConfig.
const (
	// MaxRetries is the most retries of an interrupt request.
	// (ETSI TS 102 361-2 clause 6.3.5)
	MaxRetries = 255
	// MaxRepeats is the most standalone RC bursts of one try.
	// (ETSI TS 102 361-2 clause 6.3.5)
	MaxRepeats = 8
)

// InterrupterConfig configures NewInterrupter. A nil config means the zero
// InterrupterConfig. (ETSI TS 102 361-2 clause 6.3.5)
type InterrupterConfig struct {
	// ColorCode is the color code of the EMB of the standalone RC bursts.
	// The EMB also has PI set, as an RC for the other logical channel.
	// (ETSI TS 102 361-1 clause 6.4.2 and Table 9.19, ETSI TS 102 361-2
	// clause 6.3.5)
	ColorCode enums.ColorCode
	// Wait is T_RCtimer, the wait for the Terminator with LC after each
	// try. 0 means constants.TRCTimer (600 ms), and a negative value waits
	// until the terminator or Cancel. (ETSI TS 102 361-2 clause A.1)
	Wait time.Duration
	// Retries is the number of tries after the first. 0 means 1, the one
	// automatic retry of Motorola Transmit Interrupt (MOTOTRBO System
	// Planner 2.3.2), a negative value means none, and it is at most
	// MaxRetries. (ETSI TS 102 361-2 clause 6.3.5)
	Retries int
	// Repeats is the number of standalone RC bursts of each try, 60 ms
	// apart. 0 means 1; at most MaxRepeats. (ETSI TS 102 361-2 clause 6.3.5)
	Repeats int
	// Scratch, when set, is the Burst that Burst decodes into. Machines
	// that run on one goroutine can share one. Nil means the Interrupter
	// makes its own. (ETSI TS 102 361-2 clause 6.3.5)
	Scratch *layer2.Burst
}

// InterrupterState is the state of an Interrupter.
// (ETSI TS 102 361-2 clause 6.3.5)
type InterrupterState uint8

// The InterrupterState values.
const (
	// InterrupterIdle runs no interrupt request.
	// (ETSI TS 102 361-2 clause 6.3.5)
	InterrupterIdle InterrupterState = iota
	// InterrupterSending sends the standalone RC bursts of a try.
	// (ETSI TS 102 361-2 clause 6.3.5)
	InterrupterSending
	// InterrupterWaiting waits T_RCtimer for the Terminator with LC.
	// (ETSI TS 102 361-2 clause 6.3.5)
	InterrupterWaiting
)

// String returns the name of the state. It allocates only for a value
// without a name.
func (s InterrupterState) String() string {
	switch s {
	case InterrupterIdle:
		return idleName
	case InterrupterSending:
		return "Sending"
	case InterrupterWaiting:
		return "Waiting"
	}
	return fmt.Sprintf("InterrupterState(%d)", uint8(s))
}

// Interrupter is the radio that interrupts the voice call of another
// radio: it sends a Tier 2 RC command in standalone RC bursts on the
// reverse channel of the call, waits T_RCtimer for the Terminator with LC
// of the call, and tries again up to InterrupterConfig.Retries times. The
// caller sends the bursts on the reverse channel and feeds the bursts of
// the call to Burst. The zero Interrupter does nothing: Interrupt returns
// dmrgo.ErrZeroValue. Make it with NewInterrupter. An Interrupter is not
// safe for concurrent use and must not be copied. (ETSI TS 102 361-2
// clause 6.3.5)
type Interrupter struct {
	cc      enums.ColorCode
	wait    time.Duration
	retries int
	repeats int

	state     InterrupterState
	rc        layer2.RawBurst
	tries     int
	sent      int
	due       time.Duration
	waitUntil time.Duration

	b        *layer2.Burst
	opts     layer2.DecodeOptions
	dataType enums.DataType
	noCopy   noCopy
}

// NewInterrupter returns an Interrupter for cfg. A nil cfg means the zero
// InterrupterConfig. It returns a *dmrgo.ConfigError for a color code
// above 15, Retries above MaxRetries, or Repeats out of range. It allocates
// the Interrupter, and its
// Burst unless cfg.Scratch is set. (ETSI TS 102 361-2 clause 6.3.5)
func NewInterrupter(cfg *InterrupterConfig) (*Interrupter, error) {
	var conf InterrupterConfig
	if cfg != nil {
		conf = *cfg
	}
	cfg = &conf
	switch {
	case cfg.ColorCode > 15:
		return nil, errColorCode
	case cfg.Retries > MaxRetries:
		return nil, errRetries
	case cfg.Repeats < 0 || cfg.Repeats > MaxRepeats:
		return nil, errRepeats
	}
	i := &Interrupter{cc: cfg.ColorCode, wait: cfg.Wait, retries: cfg.Retries, repeats: max(cfg.Repeats, 1), b: cfg.Scratch}
	switch {
	case i.retries == 0:
		i.retries = 1
	case i.retries < 0:
		i.retries = 0
	}
	if i.wait == 0 {
		i.wait = constants.TRCTimer
	}
	if i.b == nil {
		i.b = layer2.NewBurst()
	}
	return i, nil
}

// Reset ends any interrupt request without an output. It keeps the config.
// (ETSI TS 102 361-2 clause 6.3.5)
func (i *Interrupter) Reset() {
	if i == nil {
		return
	}

	i.state = InterrupterIdle
}

// State returns the state of the Interrupter.
// (ETSI TS 102 361-2 clause 6.3.5)
func (i *Interrupter) State() InterrupterState {
	if i == nil {
		return 0
	}

	return i.state
}

// Interrupt starts an interrupt request with the RC command cmd, whose
// first standalone RC burst is due at now: enums.RCCeaseTransmissionRequest
// for a radio that is party to the call, enums.RCCeaseTransmissionCommand
// for an emergency call. It returns ErrInterruptBusy while a request runs, and a
// *dmrgo.ConfigError for another command. (ETSI TS 102 361-2 clauses 6.3.3 and
// 6.3.4)
func (i *Interrupter) Interrupt(now time.Duration, cmd enums.RCCommand) error {
	if i == nil {
		return dmrgo.ErrNilReceiver
	}
	if i.b == nil {
		return dmrgo.ErrZeroValue
	}

	if i.state != InterrupterIdle {
		return ErrInterruptBusy
	}
	if cmd != enums.RCCeaseTransmissionCommand && cmd != enums.RCCeaseTransmissionRequest {
		return errCommand
	}
	d, err := layer2.EncodeEmbeddedRC(&pdu.ReverseChannel{RCCommand: cmd})
	if err != nil {
		return errCommand
	}
	i.rc = layer2.RawBurst{}
	_ = i.rc.SetSync(enums.SyncMSSourcedRC)
	_ = i.rc.SetStandaloneRCEMB(pdu.EmbeddedSignalling{ColorCode: i.cc, PreemptionAndPowerControlIndicator: true, LCSS: enums.LCSSSingleFragment})
	i.rc.SetStandaloneRCData(d)
	i.tries = 0
	i.try(now)
	return nil
}

func (i *Interrupter) try(at time.Duration) {
	i.state, i.sent, i.due = InterrupterSending, 0, at
	i.tries++
}

// Cancel ends the interrupt request, and appends OutputInterruptCanceled
// at now to dst while one runs. It does not run the timers. It allocates
// only when dst has no room for MaxOutputs outputs.
// (ETSI TS 102 361-2 clause 6.3.5)
func (i *Interrupter) Cancel(now time.Duration, dst []Output) []Output {
	if i == nil || i.state == InterrupterIdle {
		return dst
	}

	i.state = InterrupterIdle
	dst, _ = appendOutput(dst, OutputInterruptCanceled, now)
	return dst
}

// Burst decodes the burst raw of the call with what the Hint h says about
// it, runs it through the Interrupter as Decoded does, and appends the
// outputs to dst. A nil raw, a transport frame without a burst, only runs
// the timers, as Lost does. It allocates only when dst has no room for
// MaxOutputs outputs. (ETSI TS 102 361-2 clause 6.3.5)
func (i *Interrupter) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Output) []Output {
	if i == nil || i.b == nil {
		return dst
	}

	i.opts = layer2.DecodeOptions{Kind: h.Kind, VoiceBurst: enums.VoiceBurstUnknown}
	if h.Kind == layer2.KindVoice {
		i.opts.VoiceBurst = h.VoiceBurst
	}
	if h.HasDataType {
		i.dataType = h.DataType
		i.opts.DataType = &i.dataType
	}
	b := i.b
	if raw == nil || b.DecodeFromBytesWith(*raw, &i.opts) != nil {
		b = nil
	}
	return i.Decoded(now, b, h, dst)
}

// Decoded takes the decoded burst b of the call, which may be nil, and
// appends the outputs to dst: OutputInterrupted when b is a Terminator
// with LC after the first standalone RC burst went out. It allocates only
// when dst has no room for MaxOutputs outputs.
// (ETSI TS 102 361-2 clause 6.3.5)
func (i *Interrupter) Decoded(now time.Duration, b *layer2.Burst, _ layer2.Hint, dst []Output) []Output {
	if i == nil {
		return dst
	}

	dst = i.Tick(now, dst)
	if i.state == InterrupterIdle || i.tries == 1 && i.state == InterrupterSending && i.sent == 0 {
		return dst
	}
	if b == nil || b.Kind != layer2.KindData || b.SlotType.FEC.Uncorrectable || b.SlotType.DataType != enums.DataTypeTerminatorWithLC {
		return dst
	}
	i.state = InterrupterIdle
	dst, _ = appendOutput(dst, OutputInterrupted, now)
	return dst
}

// Tick runs the timers that are due at now and appends their outputs to
// dst: at most one standalone RC burst, with Position
// enums.VoiceBurstUnknown, and OutputInterruptFailed when T_RCtimer ran out
// after the last try. A Tick before Deadline does nothing. It allocates
// only when dst has no room for MaxOutputs outputs.
// (ETSI TS 102 361-2 clause 6.3.5)
func (i *Interrupter) Tick(now time.Duration, dst []Output) []Output {
	if i == nil {
		return dst
	}

	if i.state == InterrupterWaiting && i.wait >= 0 && now >= i.waitUntil {
		if i.tries > i.retries {
			i.state = InterrupterIdle
			dst, _ = appendOutput(dst, OutputInterruptFailed, i.waitUntil)
			return dst
		}
		i.try(max(i.waitUntil, now))
	}
	if i.state != InterrupterSending || now < i.due {
		return dst
	}
	var o *Output
	dst, o = appendOutput(dst, OutputBurst, i.due)
	o.Burst, o.Hint = i.rc, layer2.Hint{Kind: layer2.KindReverseChannel}
	i.sent++
	if i.sent < i.repeats {
		i.due += slotPeriod
		return dst
	}
	i.state, i.waitUntil = InterrupterWaiting, i.due+i.wait
	return dst
}

// Deadline returns the time at which the Interrupter needs a Tick. ok is
// false when no timer runs. (ETSI TS 102 361-2 clause 6.3.5)
func (i *Interrupter) Deadline() (at time.Duration, ok bool) {
	if i == nil {
		return 0, false
	}

	switch i.state {
	case InterrupterSending:
		return i.due, true
	case InterrupterWaiting:
		return i.waitUntil, i.wait >= 0
	case InterrupterIdle:
	}
	return 0, false
}
