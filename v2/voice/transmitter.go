package voice

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/pcg"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// MaxOutputs is the most outputs that one call of a Transmitter or
// Interrupter method appends: one burst, and OutputEnded after the last
// terminator.
const MaxOutputs = 2

// Limits and defaults of TransmitterConfig.
const (
	// MaxHeaders is the most Voice LC Headers before burst A.
	MaxHeaders = 8
	// MaxTerminators is the most Terminators with LC at the end of a call.
	MaxTerminators = 64
	// DefaultQueue is the number of vocoder slots that a Transmitter
	// buffers when TransmitterConfig.Queue is 0: three bursts.
	DefaultQueue = 9
	// MaxQueue is the most vocoder slots that a Transmitter buffers.
	MaxQueue = 3 * 64
	// DefaultIdleSearch is the idle search timer T_IdleSrch of a polite
	// call, the most that ETSI TS 102 361-1 clause F.1 allows.
	DefaultIdleSearch = 540 * time.Millisecond
	// DefaultHoldoffMax is the upper end of the random holdoff before a new
	// channel authorization try, the recommended maximum T_Holdoff of ETSI
	// TS 102 361-1 clause F.1.
	DefaultHoldoffMax = time.Second
)

// TransmitterConfig configures NewTransmitter. A nil config means the zero
// TransmitterConfig.
type TransmitterConfig struct {
	// ColorCode is the color code of the slot types and EMBs.
	ColorCode enums.ColorCode
	// Source selects the SYNC patterns: layer2.SyncSourceBS, the zero
	// value, for a repeater outbound, layer2.SyncSourceMS for a radio.
	Source layer2.SyncSource
	// Silence is the 72-bit slot that fills a burst when no voice is
	// queued, and pads the last superframe, packed as
	// layer2.RawBurst.VoiceSlots returns it. The zero value means
	// vocoder.AMBESilenceSlot(). A Codec2 caller sets its own.
	Silence [9]byte
	// Headers is the number of Voice LC Headers before burst A. 0 means
	// 1, as ETSI TS 102 361-1 clause 5.1.2.2 gives; at most MaxHeaders.
	// MMDVMHost sends 3 as a duplex repeater.
	Headers int
	// Terminators is the number of Terminators with LC at the end of a
	// call. 0 means 1; at most MaxTerminators.
	Terminators int
	// Timeout is the transmit timeout T_TO. 0 means constants.TTO (180 s),
	// and a negative value turns it off (ETSI TS 102 361-2 clause 6.1).
	Timeout time.Duration
	// Queue is the number of vocoder slots buffered. 0 means DefaultQueue;
	// at most MaxQueue. It must hold Headers+1 bursts, so that a relayed
	// late entry can send its headers first.
	Queue int
	// Lead makes Tick emit a burst this long before its Due time, so that
	// an RF FIFO can be filled ahead. 0 means at Due.
	Lead time.Duration
	// Access is the channel access policy of Start. A polite call waits
	// up to IdleSearch for Channel to report an idle channel and ends with
	// EndDenied when it does not; an impolite call starts at once. Relay
	// ignores it. The zero value is polite to own color code. (ETSI TS 102
	// 361-1 clauses 5.2.1.6 and 5.2.2)
	Access enums.Access
	// IdleSearch is T_IdleSrch, the wait for an idle channel before a
	// polite call. 0 means DefaultIdleSearch, and a negative value denies
	// the call at once. (ETSI TS 102 361-1 clause 5.2.2.2.5)
	IdleSearch time.Duration
	// Interruptible makes Interrupt end the call on a Cease Transmission
	// Request. A Cease Transmission Command ends it whatever this says.
	// (ETSI TS 102 361-2 clauses 6.3.3 and 6.3.4)
	Interruptible bool
	// ChanAuth is T_MS_ChanAuth, the wait for the BS to repeat the ChanAuth
	// PDU of a call that starts in hang time, CallOptions.HangTime. 0 means
	// no channel authorization; constants.TMSChanAuth and
	// constants.TMSChanAuthMultiSite are the recommended values. (ETSI TS
	// 102 361-2 clause 5.1.1.6.2)
	ChanAuth time.Duration
	// HoldoffMax is the upper end of the random holdoff before a new
	// channel authorization try. 0 means DefaultHoldoffMax, and a negative
	// value means no holdoff. (ETSI TS 102 361-2 clause 5.1.1.6.2)
	HoldoffMax time.Duration
	// Seed seeds the generator of the random holdoff, so that the same
	// inputs give the same outputs. (ETSI TS 102 361-2 clause 5.1.1.6.2)
	Seed uint64
	// Scratch, when set, is the Burst that Burst decodes into. Machines
	// that run on one goroutine can share one. Nil means the Transmitter
	// makes its own when ChanAuth is set. (ETSI TS 102 361-2 clause
	// 5.1.1.6.2)
	Scratch *layer2.Burst
}

// CallOptions are per-call settings of Transmitter.Start.
type CallOptions struct {
	// PIHeader, when set, is sent after the Voice LC Headers.
	PIHeader *pdu.PIHeader
	// TalkerAlias, when not empty, is sent in the embedded LC of every
	// other superframe, in TalkerAliasFormat.
	TalkerAlias string
	// TalkerAliasFormat is the character format of TalkerAlias. The zero
	// value is layer3.TalkerAliasSevenBit.
	TalkerAliasFormat layer3.TalkerAliasDataFormat
	// GPS, when set, is sent in the embedded LC of every other superframe,
	// after the talker alias.
	GPS *pdu.FLCGPSInfo
	// InCall reports that the radio is party to the call on the channel,
	// or that the call is a response that the channel solicited. Such a
	// call starts whatever the channel state and TransmitterConfig.Access.
	// (ETSI TS 102 361-1 clauses 5.2.1.6 and 5.2.2.2.7)
	InCall bool
	// HangTime reports that the BS sends call or channel hang time bursts.
	// With TransmitterConfig.ChanAuth set, a call that is not impolite then
	// starts with the channel authorization. (ETSI TS 102 361-2 clause
	// 5.1.1.6)
	HangTime bool
}

// OutputKind is the kind of an Output.
type OutputKind uint8

// The OutputKind values.
const (
	// OutputBurst is a burst to send.
	OutputBurst OutputKind = iota + 1
	// OutputEnded is the end of the transmission.
	OutputEnded
	// OutputInterrupted is the Terminator with LC that ends the call of
	// another radio after the RC command of an Interrupter (ETSI TS 102
	// 361-2 clause 6.3.5).
	OutputInterrupted
	// OutputInterruptFailed is an interrupt request of an Interrupter
	// without a Terminator with LC after the last try (ETSI TS 102 361-2
	// clause 6.3.5).
	OutputInterruptFailed
)

// String returns the name of the kind. It allocates only for a value without
// a name.
func (k OutputKind) String() string {
	switch k {
	case OutputBurst:
		return "Burst"
	case OutputEnded:
		return "Ended"
	case OutputInterrupted:
		return "Interrupted"
	case OutputInterruptFailed:
		return "InterruptFailed"
	}
	return fmt.Sprintf("OutputKind(%d)", uint8(k))
}

// Output is one result of a Transmitter or an Interrupter. Fields that do
// not apply to Kind are zero.
type Output struct {
	// Kind is the kind of the output.
	Kind OutputKind
	// Burst is the burst to send.
	Burst layer2.RawBurst
	// Due is the time at which the burst goes on air, 60 ms after the one
	// before it.
	Due time.Duration
	// Hint is what the burst is, for a transport: KindVoice with the
	// superframe position, or KindData with the data type of a header, PI
	// Header or terminator. Its Slot is SlotUnknown.
	Hint layer2.Hint
	// Fill reports a voice burst with silence in place of voice that was
	// not queued in time, or that pads the last superframe. The fills that
	// Relay queues for lost bursts are voice and do not set it.
	Fill bool
	// Reason is why the transmission ended: EndTerminator after a normal
	// end, EndLost for a relayed call that was lost, EndTimeout after
	// T_TO.
	Reason EndReason
}

// TransmitterState is the state of a Transmitter.
type TransmitterState uint8

// The TransmitterState values.
const (
	// TransmitterIdle sends nothing.
	TransmitterIdle TransmitterState = iota
	// TransmitterHeader sends the Voice LC Headers and the PI Header.
	TransmitterHeader
	// TransmitterVoice sends voice bursts.
	TransmitterVoice
	// TransmitterEnding pads the last superframe and sends the
	// terminators.
	TransmitterEnding
	// TransmitterTimedOut has ended the call after T_TO and waits for
	// Stop, the release of the PTT (ETSI TS 102 361-2 clause 6.1).
	TransmitterTimedOut
	// TransmitterAccess waits for Channel to report an idle channel
	// before a polite call (ETSI TS 102 361-1 clause 5.2.2).
	TransmitterAccess
	// TransmitterChanAuth sends the ChanAuth PDU of a call in hang time and
	// waits T_MS_ChanAuth for the BS to repeat it (ETSI TS 102 361-2 clause
	// 5.1.1.6.2).
	TransmitterChanAuth
)

// String returns the name of the state. It allocates only for a value
// without a name.
func (s TransmitterState) String() string {
	switch s {
	case TransmitterIdle:
		return idleName
	case TransmitterHeader:
		return "Header"
	case TransmitterVoice:
		return "Voice"
	case TransmitterEnding:
		return "Ending"
	case TransmitterTimedOut:
		return "TimedOut"
	case TransmitterAccess:
		return "Access"
	case TransmitterChanAuth:
		return "ChanAuth"
	}
	return fmt.Sprintf("TransmitterState(%d)", uint8(s))
}

// Transmitter makes the bursts of one voice transmission on one logical
// channel, one per Tick, 60 ms apart. In source mode, Start begins a call
// and Frame, Frames, Slot and Slots queue its voice. In relay mode, Relay
// takes the events of a Receiver and sends the call again with the local
// color code and SYNC. Make it with NewTransmitter. A Transmitter is not
// safe for concurrent use and must not be copied.
type Transmitter struct {
	cc          enums.ColorCode
	source      layer2.SyncSource
	silence     [9]byte
	headers     int
	terminators int
	timeout     time.Duration
	lead        time.Duration
	access      enums.Access
	idleSearch  time.Duration
	interrupt   bool
	busy        bool
	waitUntil   time.Duration
	chanAuth    time.Duration
	holdoffMax  time.Duration
	rng         pcg.Source
	scratch     *layer2.Burst
	dopts       layer2.DecodeOptions
	dataType    enums.DataType
	authWaiting bool
	busySeen    bool

	vt    layer2.VoiceTransmitter
	vopts layer2.VoiceTransmitterOptions
	pi    pdu.PIHeader
	lc    pdu.FullLinkControl

	state      TransmitterState
	call       Call
	start      time.Duration
	due        time.Duration
	sent       int
	pos        int
	superframe int
	timedOut   bool
	stopping   bool
	ended      bool
	reason     EndReason
	piSent     bool
	underruns  int

	queue  [][9]byte
	head   int
	length int
	slots  [layer2.VoiceSlotsPerBurst][9]byte

	ta        [4]pdu.FullLinkControl
	nTA       int
	gpsLC     pdu.FullLinkControl
	nExtras   int
	nextExtra int

	relay      bool
	last       [9]byte
	hasLast    bool
	frags      [4][4]byte
	fragNext   int
	carry      [4][4]byte
	hasCarry   bool
	relayPI    layer2.RawBurst
	hasRelayPI bool
	next       Call
	hasNext    bool
	oldSlots   int
	nextPI     layer2.RawBurst
	hasNextPI  bool
	nextStop   bool
	nextReason EndReason
	dropped    int
}

// NewTransmitter returns a Transmitter for cfg. A nil cfg means the zero
// TransmitterConfig. It returns a *dmrgo.ConfigError for a field out of
// range. It allocates the Transmitter and its queue, 72 bytes for each slot.
func NewTransmitter(cfg *TransmitterConfig) (*Transmitter, error) {
	var conf TransmitterConfig
	if cfg != nil {
		conf = *cfg
	}
	cfg = &conf
	t := &Transmitter{cc: cfg.ColorCode, source: cfg.Source, silence: cfg.Silence, headers: cfg.Headers,
		terminators: cfg.Terminators, timeout: cfg.Timeout, lead: cfg.Lead, access: cfg.Access,
		idleSearch: cfg.IdleSearch, interrupt: cfg.Interruptible, chanAuth: cfg.ChanAuth, holdoffMax: cfg.HoldoffMax,
		rng: pcg.NewStream(cfg.Seed, pcgStream), scratch: cfg.Scratch}
	if t.holdoffMax == 0 {
		t.holdoffMax = DefaultHoldoffMax
	}
	if t.headers == 0 {
		t.headers = 1
	}
	if t.terminators == 0 {
		t.terminators = 1
	}
	if t.timeout == 0 {
		t.timeout = constants.TTO
	}
	if t.idleSearch == 0 {
		t.idleSearch = DefaultIdleSearch
	}
	if t.silence == [9]byte{} {
		t.silence = vocoder.AMBESilenceSlot()
	}
	queue := cfg.Queue
	if queue == 0 {
		queue = DefaultQueue
	}
	switch _, _, ok := cfg.Source.SyncPatterns(); {
	case cfg.ColorCode > 15:
		return nil, errColorCode
	case cfg.Access > enums.AccessImpolite:
		return nil, errAccess
	case cfg.ChanAuth < 0:
		return nil, errChanAuth
	case !ok:
		return nil, errSource
	case t.headers < 0 || t.headers > MaxHeaders:
		return nil, errHeaders
	case t.terminators < 0 || t.terminators > MaxTerminators:
		return nil, errTerminators
	case queue < layer2.VoiceSlotsPerBurst*(t.headers+1) || queue > MaxQueue:
		return nil, errQueue
	}
	t.queue = make([][9]byte, queue)
	if t.chanAuth > 0 && t.scratch == nil {
		t.scratch = layer2.NewBurst()
	}
	return t, nil
}

// Reset ends any transmission without an output, empties the queue and
// forgets the channel state. It keeps the config.
func (t *Transmitter) Reset() {
	if t == nil {
		return
	}

	t.state, t.ended, t.timedOut, t.hasNext, t.relay, t.stopping = TransmitterIdle, false, false, false, false, false
	t.busy = false
	t.head, t.length, t.dropped, t.reason = 0, 0, 0, 0
}

// State returns the state of the Transmitter.
func (t *Transmitter) State() TransmitterState {
	if t == nil {
		return 0
	}

	return t.state
}

// Room returns the number of vocoder slots that the queue can take.
func (t *Transmitter) Room() int {
	if t == nil {
		return 0
	}

	return len(t.queue) - t.length
}

// Dropped returns the number of relayed voice bursts that the Transmitter
// dropped: voice before the LC was known, and voice that did not fit in
// the queue.
func (t *Transmitter) Dropped() int {
	if t == nil {
		return 0
	}

	return t.dropped
}

// Start begins the call c, whose first burst is due at the time at. The
// LC is c.LC when it matches the other fields of c, else it is built from
// them. opts may be nil. A polite call on a busy channel waits in
// TransmitterAccess, as TransmitterConfig.Access says, and a call in hang
// time starts in TransmitterChanAuth, as CallOptions.HangTime says. Start
// returns ErrBusy while a transmission runs, and a *dmrgo.ConfigError when c
// does not make an LC.
func (t *Transmitter) Start(at time.Duration, c Call, opts *CallOptions) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	if t.state != TransmitterIdle {
		return ErrBusy
	}
	t.relay = false
	if err := t.begin(at, &c, opts, false); err != nil {
		return err
	}
	if t.chanAuth > 0 && opts != nil && opts.HangTime && t.access != enums.AccessImpolite {
		t.state, t.authWaiting = TransmitterChanAuth, false
		return nil
	}
	if t.busy && t.access != enums.AccessImpolite && (opts == nil || !opts.InCall) {
		t.state, t.waitUntil = TransmitterAccess, at
		if t.idleSearch > 0 {
			t.waitUntil += t.idleSearch
		}
	}
	return nil
}

// Channel reports whether the logical channel is busy, from the activity
// that the caller sees on it, such as the AT bit of the CACH, the carrier,
// or a call with its own color code. A call in TransmitterAccess starts
// when the channel becomes idle. Until the first report the channel counts
// as idle. (ETSI TS 102 361-1 clause 5.2.1.2)
func (t *Transmitter) Channel(now time.Duration, busy bool) {
	if t == nil {
		return
	}

	t.busy = busy
	if busy || t.state != TransmitterAccess {
		return
	}
	t.state = TransmitterHeader
	if now > t.due {
		t.start, t.due = now, now
	}
}

// Interrupt takes a Tier 2 RC command that the reverse channel of the call
// carried, and reports whether the Transmitter ends the call for it. A
// Cease Transmission Command always ends a call on air, and a Cease
// Transmission Request ends it when TransmitterConfig.Interruptible is
// set. The queued voice is dropped, the last superframe is padded with
// silence, and the terminators follow, so that the radio that sent the
// command sees the Terminator with LC; OutputEnded has EndInterrupted. An
// RC PDU whose CRC fails, a reserved command, a relayed call and a call
// that is not on air are ignored. (ETSI TS 102 361-2 clause 6.3)
func (t *Transmitter) Interrupt(rc pdu.ReverseChannel) bool {
	if t == nil {
		return false
	}

	if rc.FEC.Uncorrectable || t.relay {
		return false
	}
	switch rc.RCCommand {
	case enums.RCCeaseTransmissionCommand:
	case enums.RCCeaseTransmissionRequest:
		if !t.interrupt {
			return false
		}
	default:
		return false
	}
	switch t.state {
	case TransmitterHeader, TransmitterVoice:
		t.dropOwn()
		t.stopping, t.reason = true, EndInterrupted
		return true
	case TransmitterEnding:
		if !t.timedOut {
			t.dropOwn()
			t.reason = EndInterrupted
		}
		return true
	case TransmitterIdle, TransmitterTimedOut, TransmitterAccess, TransmitterChanAuth:
	}
	return false
}

func (t *Transmitter) begin(at time.Duration, c *Call, opts *CallOptions, keep bool) error {
	if err := t.setLC(c); err != nil {
		return err
	}
	t.vopts = layer2.VoiceTransmitterOptions{LC: t.lc, ColorCode: t.cc, Source: t.source, Silence: t.silence}
	t.nExtras, t.nextExtra = 0, 0
	if opts != nil {
		if opts.PIHeader != nil {
			t.pi = *opts.PIHeader
			t.vopts.PIHeader = &t.pi
		}
		if err := t.setExtras(opts); err != nil {
			return err
		}
	}
	if err := t.vt.Reset(&t.vopts); err != nil {
		return err
	}
	t.call = *c
	t.state, t.start, t.due, t.sent = TransmitterHeader, at, at, 0
	t.pos, t.superframe, t.timedOut, t.ended, t.stopping = 0, 0, false, false, false
	t.reason, t.piSent, t.underruns = 0, false, 0
	t.fragNext, t.hasCarry, t.hasLast, t.hasRelayPI = 0, false, false, false
	if !keep {
		t.head, t.length = 0, 0
	}
	return nil
}

func (t *Transmitter) setLC(c *Call) error {
	lc := c.LC
	if got, ok := callOf(lc, OriginHeader); !ok || !got.sameParties(c) || got.Options != c.Options || got.FID != c.FID {
		var err error
		if lc, err = pdu.NewVoiceLCBytes(c.Group, c.Target, c.Source, c.Options); err != nil {
			return err
		}
		lc.SetFID(c.FID)
	}
	t.lc = pdu.DecodeFullLinkControlBytes(lc)
	c.LC = lc
	return nil
}

func (t *Transmitter) setExtras(opts *CallOptions) error {
	t.nTA = 0
	if opts.TalkerAlias != "" {
		n, err := pdu.EncodeTalkerAliasInto(&t.ta, opts.TalkerAlias, opts.TalkerAliasFormat)
		if err != nil {
			return errTalkerAlias
		}
		t.nTA = n
	}
	t.nExtras = t.nTA
	if opts.GPS != nil {
		t.gpsLC = pdu.FullLinkControl{}
		if err := t.gpsLC.SetGPSInfo(*opts.GPS); err != nil {
			return errGPS
		}
		t.nExtras++
	}
	return nil
}

func (t *Transmitter) extra(i int) *pdu.FullLinkControl {
	if i < t.nTA {
		return &t.ta[i]
	}
	return &t.gpsLC
}

// Frame queues one AMBE+2 frame. It returns ErrQueueFull when Room is 0,
// ErrTimedOut after T_TO, ErrNotActive when no call takes voice, and the
// errors of vocoder.EncodeAMBE72.
func (t *Transmitter) Frame(f vocoder.AMBEFrame) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	slot, err := vocoder.EncodeAMBE72(f)
	if err != nil {
		return err
	}
	return t.Slot(slot)
}

// Frames queues AMBE+2 frames as Frame does. On an error the frames
// before the failing one stay queued.
func (t *Transmitter) Frames(f ...vocoder.AMBEFrame) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	for i := range f {
		if err := t.Frame(f[i]); err != nil {
			return err
		}
	}
	return nil
}

// Slot queues one 72-bit vocoder slot of any codec, packed as
// layer2.RawBurst.VoiceSlots returns it, as Frame does.
func (t *Transmitter) Slot(s [9]byte) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	switch {
	case t.state == TransmitterTimedOut || t.timedOut:
		return ErrTimedOut
	case t.state != TransmitterHeader && t.state != TransmitterVoice && t.state != TransmitterAccess && t.state != TransmitterChanAuth || t.stopping:
		return ErrNotActive
	case t.length == len(t.queue):
		return ErrQueueFull
	}
	t.queue[(t.head+t.length)%len(t.queue)] = s
	t.length++
	return nil
}

// Slots queues 72-bit vocoder slots as Slot does.
func (t *Transmitter) Slots(s ...[9]byte) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	for i := range s {
		if err := t.Slot(s[i]); err != nil {
			return err
		}
	}
	return nil
}

func (t *Transmitter) push(s *[9]byte) {
	t.queue[(t.head+t.length)%len(t.queue)] = *s
	t.length++
}

// Stop ends the call: the queued voice is sent, the last superframe is
// padded with silence to burst F, and the terminators follow. After T_TO,
// Stop is the release of the PTT, and the next Tick appends OutputEnded. A
// call in TransmitterAccess or TransmitterChanAuth ends without an output.
func (t *Transmitter) Stop() {
	if t == nil {
		return
	}

	switch t.state {
	case TransmitterAccess, TransmitterChanAuth:
		t.state, t.head, t.length = TransmitterIdle, 0, 0
	case TransmitterHeader, TransmitterVoice:
		t.stopping = true
	case TransmitterTimedOut:
		t.ended = true
	case TransmitterEnding:
		t.ended = t.ended || t.timedOut
	case TransmitterIdle:
	}
}

// Deadline returns the time at which the Transmitter needs a Tick: the Due
// time of the next burst minus TransmitterConfig.Lead. ok is false when it
// sends nothing.
func (t *Transmitter) Deadline() (at time.Duration, ok bool) {
	if t == nil {
		return 0, false
	}

	switch {
	case t.state == TransmitterIdle, t.state == TransmitterTimedOut && !t.ended:
		return 0, false
	case t.state == TransmitterAccess, t.state == TransmitterChanAuth && t.authWaiting:
		return t.waitUntil, true
	}
	return t.due - t.lead, true
}

// Tick appends the next burst when now has reached its Due time minus
// TransmitterConfig.Lead, and OutputEnded after the last terminator, or
// with EndDenied for a polite call that the channel denied for
// T_IdleSrch. A Tick before Deadline appends nothing. Each Tick appends at
// most one burst; a late caller calls Tick until Deadline is in the
// future. It appends to dst, and allocates only when dst has no room for
// MaxOutputs outputs.
func (t *Transmitter) Tick(now time.Duration, dst []Output) []Output {
	if t == nil {
		return dst
	}

	at, ok := t.Deadline()
	if !ok || now < at {
		return dst
	}
	if t.state == TransmitterTimedOut {
		return t.finish(dst, EndTimeout)
	}
	if t.state == TransmitterAccess {
		t.due, t.head, t.length = t.waitUntil, 0, 0
		return t.finish(dst, EndDenied)
	}
	if t.state == TransmitterChanAuth {
		return t.authTick(dst)
	}
	if t.state == TransmitterHeader && !t.headersDone() {
		var o *Output
		dst, o = appendOutput(dst, OutputBurst, t.due)
		t.header(o)
		t.due += slotPeriod
		return dst
	}
	if t.state == TransmitterVoice && t.stopping {
		t.state = TransmitterEnding
	}
	var o *Output
	switch t.state {
	case TransmitterVoice, TransmitterEnding:
		if t.state == TransmitterEnding && t.pos == 0 && (t.ownSlots() == 0 || t.timedOut) {
			t.dropOwn()
			dst, o = appendOutput(dst, OutputBurst, t.due)
			dst = t.terminator(o, dst)
		} else {
			dst, o = appendOutput(dst, OutputBurst, t.due)
			t.voice(o)
		}
	case TransmitterIdle, TransmitterHeader, TransmitterTimedOut, TransmitterAccess, TransmitterChanAuth:
	}
	t.due += slotPeriod
	return dst
}

func (t *Transmitter) ownSlots() int {
	if t.hasNext {
		return t.oldSlots
	}
	return t.length
}

func (t *Transmitter) dropOwn() {
	n := t.ownSlots()
	t.head, t.length = (t.head+n)%len(t.queue), t.length-n
	t.oldSlots = 0
}

func (t *Transmitter) header(o *Output) {
	switch {
	case t.sent >= t.headers && t.piPending() && t.vopts.PIHeader != nil:
		o.Burst, _ = t.vt.PIHeader()
		o.Hint = dataHint(enums.DataTypePIHeader)
		t.piSent = true
	case t.sent >= t.headers && t.piPending():
		o.Burst, o.Hint = t.relayPI, dataHint(enums.DataTypePIHeader)
		t.piSent = true
	default:
		o.Burst, o.Hint = t.vt.Header(), dataHint(enums.DataTypeVoiceLCHeader)
	}
	t.sent++
}

func (t *Transmitter) headersDone() bool {
	if t.sent < t.headers || t.piPending() {
		return false
	}
	if t.relay && t.length == 0 && !t.stopping && t.sent < MaxHeaders {
		return false
	}
	t.state, t.sent = TransmitterVoice, 0
	return true
}

func (t *Transmitter) piPending() bool {
	return (t.vopts.PIHeader != nil || t.hasRelayPI) && !t.piSent
}

func (t *Transmitter) voice(o *Output) {
	if t.pos == 0 {
		t.startSuperframe()
	}
	silent := 0
	for i := range t.slots {
		if t.ownSlots() > 0 {
			t.slots[i] = t.queue[t.head]
			t.head = (t.head + 1) % len(t.queue)
			t.length--
			if t.hasNext {
				t.oldSlots--
			}
		} else {
			t.slots[i], o.Fill = t.silence, true
			silent++
		}
	}
	if silent == len(t.slots) && t.state == TransmitterVoice {
		t.underruns++
	}
	if b, err := t.vt.AddBurst(t.slots); err == nil {
		o.Burst = b
	}
	o.Hint.Kind = layer2.KindVoice
	o.Hint.VoiceBurst, _ = enums.VoiceBurstAt(t.pos)
	t.pos = (t.pos + 1) % layer2.VoiceBurstsPerSuperframe
	if !t.timedOut && t.timeout >= 0 && o.Due+slotPeriod-t.start >= t.timeout {
		t.ended = t.state == TransmitterEnding
		t.state, t.timedOut = TransmitterEnding, true
	}
}

func (t *Transmitter) startSuperframe() {
	switch {
	case t.hasCarry:
		_ = t.vt.SetEmbeddedFragments(t.carry)
		t.hasCarry = false
	case t.nExtras > 0 && t.superframe%2 == 1:
		_ = t.vt.SetEmbeddedLC(*t.extra(t.nextExtra))
		t.nextExtra = (t.nextExtra + 1) % t.nExtras
	}
	t.superframe++
}

func (t *Transmitter) terminator(o *Output, out []Output) []Output {
	o.Burst, o.Hint = t.vt.Terminator(), dataHint(enums.DataTypeTerminatorWithLC)
	t.sent++
	switch {
	case t.timedOut && t.ended:
		return t.finish(out, EndTimeout)
	case t.timedOut:
		t.state = TransmitterTimedOut
		return out
	case t.sent < t.terminators:
		return out
	}
	reason := EndTerminator
	if t.reason != 0 {
		reason = t.reason
	}
	return t.finish(out, reason)
}

func (t *Transmitter) finish(out []Output, reason EndReason) []Output {
	var o *Output
	out, o = appendOutput(out, OutputEnded, t.due)
	o.Reason = reason
	t.state, t.reason, t.ended, t.timedOut, t.stopping = TransmitterIdle, 0, false, false, false
	if !t.hasNext {
		return out
	}
	t.hasNext = false
	if t.begin(t.due, &t.next, nil, true) != nil {
		t.head, t.length = 0, 0
		return out
	}
	t.relay = true
	t.relayPI, t.hasRelayPI, t.hasNextPI = t.nextPI, t.hasNextPI, false
	if t.nextStop {
		t.stopping, t.reason, t.nextStop = true, t.nextReason, false
	}
	return out
}

// Relay takes an event of a Receiver and sends the call again: a
// CallStart starts it with its headers, voice is queued, a PI Header is
// sent after the headers, and a CallEnd ends it. Voice that comes before
// its CallStart is dropped and counted, as is voice that does not fit in
// the queue. A lost burst becomes a fill: the first repeats the last
// vocoder slot, later ones are silence. The embedded LC of each complete
// received superframe is sent in the next superframe that starts, and the
// call LC when none was received. It returns ErrBusy for a CallStart while
// a call that is not ending runs, and an error for a nil e.
func (t *Transmitter) Relay(now time.Duration, e *Event) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	if e == nil {
		return errNilEvent
	}
	switch e.Kind {
	case EventCallStart:
		c := e.Call
		return t.relayStart(now, &c)
	case EventVoice:
		t.relayVoice(e)
	case EventPIHeader:
		switch {
		case t.hasNext && e.Call.sameParties(&t.next) && !t.hasNextPI:
			t.nextPI, t.hasNextPI = t.recolor(&e.Burst), true
		case t.relay && t.state == TransmitterHeader && !t.hasRelayPI:
			t.relayPI, t.hasRelayPI = t.recolor(&e.Burst), true
		}
	case EventCallEnd:
		reason := EndTerminator
		if e.Reason == EndLost || e.Reason == EndSyncLost {
			reason = EndLost
		}
		switch {
		case t.hasNext && e.Call.sameParties(&t.next):
			t.nextStop, t.nextReason = true, reason
		case t.relay && !t.stopping && (t.state == TransmitterHeader || t.state == TransmitterVoice):
			if reason == EndLost {
				t.reason = reason
			}
			t.Stop()
		}
	case EventEmbedded, EventTalkerAlias, EventSessionStart, EventSessionEnd:
	}
	return nil
}

func (t *Transmitter) recolor(b *layer2.RawBurst) layer2.RawBurst {
	r := *b
	_ = r.SetSlotType(pdu.SlotType{ColorCode: t.cc, DataType: enums.DataTypePIHeader})
	_, data, _ := t.source.SyncPatterns()
	_ = r.SetSync(data)
	return r
}

func (t *Transmitter) relayStart(now time.Duration, c *Call) error {
	switch t.state {
	case TransmitterIdle:
		t.relay = true
		return t.begin(now, c, nil, false)
	case TransmitterEnding, TransmitterHeader, TransmitterVoice:
		if t.relay && !t.timedOut && !t.hasNext && (t.stopping || t.state == TransmitterEnding) {
			t.next, t.hasNext, t.oldSlots = *c, true, t.length
			t.hasNextPI, t.nextStop = false, false
			return nil
		}
	case TransmitterTimedOut, TransmitterAccess, TransmitterChanAuth:
	}
	return ErrBusy
}

func (t *Transmitter) relayVoice(e *Event) {
	next := t.hasNext && e.Call.sameParties(&t.next)
	active := t.relay && !t.stopping && (t.state == TransmitterHeader || t.state == TransmitterVoice)
	if !next && !active || e.Call.Origin == OriginUnknown {
		t.dropped++
		return
	}
	fills := int(e.Lost)
	if !next {
		fills -= t.underruns
		t.underruns = 0
	}
	for i := range fills {
		if t.Room() < layer2.VoiceSlotsPerBurst {
			break
		}
		for range layer2.VoiceSlotsPerBurst {
			if i == 0 && t.hasLast && !next {
				t.push(&t.last)
			} else {
				t.push(&t.silence)
			}
		}
	}
	if t.Room() < layer2.VoiceSlotsPerBurst {
		t.dropped++
		return
	}
	packed := e.Burst.VoiceSlots()
	for i := range packed {
		s := &t.queue[(t.head+t.length)%len(t.queue)]
		*s = packed[i]
		t.length++
		if !next {
			t.last, t.hasLast = *s, true
		}
	}
	if !next {
		t.collect(e)
	}
}

func (t *Transmitter) collect(e *Event) {
	idx, ok := e.Position.Index()
	switch {
	case !ok || idx < 1 || idx > 4:
		t.fragNext = 0
		return
	case idx == 1:
		t.fragNext = 1
	case idx != t.fragNext+1 || e.Lost > 0:
		t.fragNext = 0
		return
	}
	t.frags[idx-1] = e.Burst.EmbeddedData()
	t.fragNext = idx
	if idx < 4 {
		return
	}
	t.fragNext = 0
	lc, res := layer2.DecodeEmbeddedLC(t.frags)
	if res.Uncorrectable {
		return
	}
	if _, voiceLC := callOf(pdu.LCBytes(lc), OriginEmbedded); !voiceLC || pdu.LCBytes(lc) == t.call.LC {
		t.carry, t.hasCarry = t.frags, true
	}
}

func dataHint(dt enums.DataType) layer2.Hint {
	return layer2.Hint{Kind: layer2.KindData, DataType: dt, HasDataType: true}
}

//go:noinline
func appendOutput(out []Output, k OutputKind, due time.Duration) ([]Output, *Output) {
	if len(out) < cap(out) {
		out = out[:len(out)+1]
		out[len(out)-1] = Output{}
	} else {
		out = append(out, Output{})
	}
	o := &out[len(out)-1]
	o.Kind, o.Due = k, due
	return out, o
}
