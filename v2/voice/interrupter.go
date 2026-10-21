package voice

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// Limits of InterrupterConfig.
const (
	// MaxRetries is the most retries of an interrupt request.
	MaxRetries = 255
	// MaxRepeats is the most standalone RC bursts of one try.
	MaxRepeats = 8
)

// InterrupterConfig configures NewInterrupter. A nil config means the zero
// InterrupterConfig.
type InterrupterConfig struct {
	// ColorCode is the color code of the EMB of the standalone RC bursts.
	ColorCode enums.ColorCode
	// Wait is T_RCtimer, the wait for the Terminator with LC after each
	// try. 0 means constants.TRCTimer (600 ms), and a negative value waits
	// until the terminator or Cancel. (ETSI TS 102 361-2 clause A.1)
	Wait time.Duration
	// Retries is the number of tries after the first. 0 means none; at
	// most MaxRetries.
	Retries int
	// Repeats is the number of standalone RC bursts of each try, 60 ms
	// apart. 0 means 1; at most MaxRepeats.
	Repeats int
	// Scratch, when set, is the Burst that Burst decodes into. Machines
	// that run on one goroutine can share one. Nil means the Interrupter
	// makes its own.
	Scratch *layer2.Burst
}

// InterrupterState is the state of an Interrupter.
type InterrupterState uint8

// The InterrupterState values.
const (
	// InterrupterIdle runs no interrupt request.
	InterrupterIdle InterrupterState = iota
	// InterrupterSending sends the standalone RC bursts of a try.
	InterrupterSending
	// InterrupterWaiting waits T_RCtimer for the Terminator with LC.
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
// the call to Burst. Make it with NewInterrupter. An Interrupter is not
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
// InterrupterConfig. It returns a *ConfigError for a color code above 15,
// or Retries or Repeats out of range. It allocates the Interrupter, and its
// Burst unless cfg.Scratch is set.
func NewInterrupter(cfg *InterrupterConfig) (*Interrupter, error) {
	if cfg == nil {
		cfg = &InterrupterConfig{}
	}
	switch {
	case cfg.ColorCode > 15:
		return nil, errColorCode
	case cfg.Retries < 0 || cfg.Retries > MaxRetries:
		return nil, errRetries
	case cfg.Repeats < 0 || cfg.Repeats > MaxRepeats:
		return nil, errRepeats
	}
	i := &Interrupter{cc: cfg.ColorCode, wait: cfg.Wait, retries: cfg.Retries, repeats: max(cfg.Repeats, 1), b: cfg.Scratch}
	if i.wait == 0 {
		i.wait = constants.TRCTimer
	}
	if i.b == nil {
		i.b = layer2.NewBurst()
	}
	return i, nil
}

// Reset ends any interrupt request without an output. It keeps the config.
func (i *Interrupter) Reset() {
	i.state = InterrupterIdle
}

// State returns the state of the Interrupter.
func (i *Interrupter) State() InterrupterState {
	return i.state
}

// Interrupt starts an interrupt request with the RC command cmd, whose
// first standalone RC burst is due at now: enums.RCCeaseTransmissionRequest
// for a radio that is party to the call, enums.RCCeaseTransmissionCommand
// for an emergency call. It returns ErrBusy while a request runs, and a
// *ConfigError for another command. (ETSI TS 102 361-2 clauses 6.3.3 and
// 6.3.4)
func (i *Interrupter) Interrupt(now time.Duration, cmd enums.RCCommand) error {
	if i.state != InterrupterIdle {
		return ErrBusy
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
	_ = i.rc.SetStandaloneRCEMB(pdu.EmbeddedSignalling{ColorCode: i.cc, LCSS: enums.LCSSSingleFragment})
	i.rc.SetStandaloneRCData(d)
	i.tries = 0
	i.try(now)
	return nil
}

func (i *Interrupter) try(at time.Duration) {
	i.state, i.sent, i.due = InterrupterSending, 0, at
	i.tries++
}

// Cancel ends the interrupt request without an output.
func (i *Interrupter) Cancel() {
	i.state = InterrupterIdle
}

// Burst decodes the burst raw of the call with what the Hint h says about
// it, runs it through the Interrupter as Decoded does, and appends the
// outputs to dst. It allocates only when dst has no room for MaxOutputs
// outputs.
func (i *Interrupter) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Output) []Output {
	i.opts = layer2.DecodeOptions{Kind: h.Kind, VoiceBurst: enums.VoiceBurstUnknown}
	if h.Kind == layer2.KindVoice {
		i.opts.VoiceBurst = h.VoiceBurst
	}
	if h.HasDataType {
		i.dataType = h.DataType
		i.opts.DataType = &i.dataType
	}
	b := i.b
	if b.DecodeFromBytesWith(*raw, &i.opts) != nil {
		b = nil
	}
	return i.Decoded(now, b, h, dst)
}

// Decoded takes the decoded burst b of the call, which may be nil, and
// appends the outputs to dst: OutputInterrupted when b is a Terminator
// with LC after the first standalone RC burst went out. It allocates only
// when dst has no room for MaxOutputs outputs.
func (i *Interrupter) Decoded(now time.Duration, b *layer2.Burst, _ layer2.Hint, dst []Output) []Output {
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
func (i *Interrupter) Tick(now time.Duration, dst []Output) []Output {
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
	o.Burst, o.Position = i.rc, enums.VoiceBurstUnknown
	i.sent++
	if i.sent < i.repeats {
		i.due += slotPeriod
		return dst
	}
	i.state, i.waitUntil = InterrupterWaiting, i.due+i.wait
	return dst
}

// Deadline returns the time at which the Interrupter needs a Tick. ok is
// false when no timer runs.
func (i *Interrupter) Deadline() (at time.Duration, ok bool) {
	switch i.state {
	case InterrupterSending:
		return i.due, true
	case InterrupterWaiting:
		return i.waitUntil, i.wait >= 0
	case InterrupterIdle:
	}
	return 0, false
}
