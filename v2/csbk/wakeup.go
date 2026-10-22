package csbk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// WakeupConfig configures NewWakeup. A nil config means the zero
// WakeupConfig.
type WakeupConfig struct {
	// ColorCode is the color code of the BS, of the slot type of the
	// BS_Dwn_Act bursts and of the outbound that ends the wakeup.
	ColorCode enums.ColorCode
	// SyncWait is T_SyncWu, the wait for the outbound after each BS_Dwn_Act.
	// 0 means DefaultSyncWait, and a negative value waits until the
	// outbound or Cancel.
	SyncWait time.Duration
	// Attempts is N_Wakeup, the most BS_Dwn_Act bursts. 0 means
	// DefaultWakeups; at most MaxRetries.
	Attempts int
	// Scratch, when set, is the Burst that Burst decodes into, as for
	// RequesterConfig.Scratch.
	Scratch *layer2.Burst
}

// WakeupState is the state of a Wakeup.
type WakeupState uint8

// The WakeupState values.
const (
	// WakeupIdle runs no wakeup.
	WakeupIdle WakeupState = iota
	// WakeupSending sends a BS_Dwn_Act at the next Tick.
	WakeupSending
	// WakeupWaiting waits T_SyncWu for the BS outbound.
	WakeupWaiting
)

// String returns the name of the state. It allocates only for a value
// without a name.
func (s WakeupState) String() string {
	switch s {
	case WakeupIdle:
		return idleName
	case WakeupSending:
		return sendingName
	case WakeupWaiting:
		return "Waiting"
	}
	return "WakeupState(" + text.Int(int64(s)) + ")"
}

// Wakeup is the MS side of the BS outbound activation: it sends BS_Dwn_Act
// with MS sourced SYNC, waits T_SyncWu for a BS sourced burst with the color
// code of the BS, and sends again up to N_Wakeup times. Make it with
// NewWakeup; a zero Wakeup does nothing, and its Wake returns
// dmrgo.ErrZeroValue. A Wakeup is not safe for concurrent use and must not be copied;
// go vet reports a copy. (ETSI TS 102 361-1 clause 5.2.2.2.4)
type Wakeup struct {
	cc       enums.ColorCode
	syncWait time.Duration
	attempts int
	d        decoder

	state     WakeupState
	csbk      pdu.CSBKBytes
	bs        uint32
	source    uint32
	sent      int
	due       time.Duration
	waitUntil time.Duration
	build     pdu.CSBK
	noCopy    noCopy
}

// NewWakeup returns a Wakeup for cfg. A nil cfg means the zero WakeupConfig.
// It returns a *dmrgo.ConfigError for a color code above 15 or Attempts out
// of range. It allocates the Wakeup, and its Burst unless cfg.Scratch is
// set.
func NewWakeup(cfg *WakeupConfig) (*Wakeup, error) {
	var conf WakeupConfig
	if cfg != nil {
		conf = *cfg
	}
	cfg = &conf
	if cfg.ColorCode > 15 {
		return nil, errColorCode
	}
	if cfg.Attempts < 0 || cfg.Attempts > MaxRetries {
		return nil, errAttempts
	}
	w := &Wakeup{cc: cfg.ColorCode, syncWait: cfg.SyncWait, attempts: cfg.Attempts}
	if w.syncWait == 0 {
		w.syncWait = DefaultSyncWait
	}
	if w.attempts == 0 {
		w.attempts = DefaultWakeups
	}
	w.d = newDecoder(cfg.Scratch)
	w.d.opts.ColorCode = &w.cc
	return w, nil
}

// Reset ends any wakeup without an output. It keeps the config.
func (w *Wakeup) Reset() {
	if w == nil {
		return
	}
	w.state = WakeupIdle
}

// State returns the state of the Wakeup.
func (w *Wakeup) State() WakeupState {
	if w == nil {
		return 0
	}
	return w.state
}

// Wake starts the wakeup of the BS with address bs by the radio source. The
// first BS_Dwn_Act goes out at the next Tick. It returns ErrBusy while a
// wakeup runs, dmrgo.ErrZeroValue on a zero Wakeup, and a
// *dmrgo.ConfigError for an address above 24 bits.
func (w *Wakeup) Wake(now time.Duration, bs, source uint32) error {
	if w == nil {
		return dmrgo.ErrNilReceiver
	}
	if w.d.b == nil {
		return dmrgo.ErrZeroValue
	}
	if w.state != WakeupIdle {
		return ErrBusy
	}
	w.build = pdu.CSBK{LastBlock: true}
	if w.build.SetBSOutboundActivation(pdu.BSOutboundActivationPDU{BSAddress: bs, SourceAddress: source}) != nil {
		return errAddress
	}
	octets, err := w.build.Encode()
	if err != nil {
		return err
	}
	w.csbk, w.bs, w.source, w.sent, w.due, w.state = pdu.CSBKBytes(octets), bs, source, 0, now, WakeupSending
	return nil
}

// OutboundSync tells the Wakeup that the BS outbound is on air, as the
// caller sees it, and appends an OutputDone with ResultAnswered to dst
// while a wakeup runs. It allocates only when dst has no room for
// MaxOutputs outputs.
func (w *Wakeup) OutboundSync(dst []Output) []Output {
	if w == nil {
		return dst
	}
	if w.state == WakeupIdle || w.sent == 0 {
		return dst
	}
	return w.done(dst, ResultAnswered)
}

// Cancel ends the wakeup, and appends an OutputDone with ResultCanceled to
// dst while one runs. It does not run the timers, so now only gives the
// shape of the other methods. It allocates only when dst has no room for
// MaxOutputs outputs.
func (w *Wakeup) Cancel(_ time.Duration, dst []Output) []Output {
	if w == nil {
		return dst
	}

	if w.state == WakeupIdle {
		return dst
	}
	return w.done(dst, ResultCanceled)
}

// Burst decodes the burst raw with what the Hint h says about it, runs it
// through the Wakeup as Decoded does, and appends the outputs to dst. A
// burst with another color code does not decode. A nil raw, a transport
// frame without a burst, only runs the timers, as Lost does. It allocates
// only when dst has no room for MaxOutputs outputs.
func (w *Wakeup) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Output) []Output {
	if w == nil {
		return dst
	}
	return w.Decoded(now, w.d.decode(raw, &h), h, dst)
}

// Decoded takes the decoded burst b, which may be nil, and appends the
// outputs to dst. A burst with a BS sourced SYNC, after the first
// BS_Dwn_Act went out, ends the wakeup with ResultAnswered. It allocates
// only when dst has no room for MaxOutputs outputs.
func (w *Wakeup) Decoded(now time.Duration, b *layer2.Burst, _ layer2.Hint, dst []Output) []Output {
	if w == nil {
		return dst
	}
	dst = w.Tick(now, dst)
	if b == nil || (b.SyncPattern != enums.SyncBSSourcedData && b.SyncPattern != enums.SyncBSSourcedVoice) {
		return dst
	}
	if b.Kind == layer2.KindData && (b.SlotType.FEC.Uncorrectable || b.SlotType.ColorCode != w.cc) {
		return dst
	}
	return w.OutboundSync(dst)
}

// Lost tells the Wakeup that the transport lost the signal. It runs the
// timers as Tick does, and allocates only when dst has no room for
// MaxOutputs outputs.
func (w *Wakeup) Lost(now time.Duration, dst []Output) []Output {
	if w == nil {
		return dst
	}
	return w.Tick(now, dst)
}

// Tick runs the timers that are due at now and appends their outputs to
// dst: a BS_Dwn_Act burst, or an OutputDone with ResultNoAnswer after the
// last T_SyncWu. A Tick before Deadline does nothing. It allocates only
// when dst has no room for MaxOutputs outputs.
func (w *Wakeup) Tick(now time.Duration, dst []Output) []Output {
	if w == nil {
		return dst
	}
	if w.state == WakeupWaiting && w.syncWait >= 0 && now >= w.waitUntil {
		if w.sent >= w.attempts {
			return w.done(dst, ResultNoAnswer)
		}
		w.state, w.due = WakeupSending, max(w.waitUntil, now)
	}
	if w.state != WakeupSending || now < w.due {
		return dst
	}
	b, err := layer2.BuildDataBurst(w.csbk, enums.DataTypeCSBK, w.cc, layer2.SyncSourceMS)
	if err != nil {
		return w.done(dst, ResultNoAnswer)
	}
	var o *Output
	dst, o = appendOutput(dst, OutputBurst)
	o.Burst, o.Due = b, w.due
	o.Hint = layer2.Hint{Kind: layer2.KindData, DataType: enums.DataTypeCSBK, HasDataType: true}
	w.sent++
	w.state, w.waitUntil = WakeupWaiting, w.due+w.syncWait
	return dst
}

// Deadline returns the time at which the Wakeup needs a Tick. ok is false
// when no timer runs.
func (w *Wakeup) Deadline() (at time.Duration, ok bool) {
	if w == nil {
		return 0, false
	}
	switch w.state {
	case WakeupSending:
		return w.due, true
	case WakeupWaiting:
		return w.waitUntil, w.syncWait >= 0
	case WakeupIdle:
	}
	return 0, false
}

func (w *Wakeup) done(out []Output, r Result) []Output {
	w.state = WakeupIdle
	var o *Output
	out, o = appendOutput(out, OutputDone)
	o.Service, o.Result, o.Target, o.Source = ServiceWakeup, r, w.bs, w.source
	return out
}
