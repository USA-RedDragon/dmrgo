package voice

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// MaxOutputs is the most outputs that one call of a Transmitter method
// appends: one burst, and OutputEnded after the last terminator.
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
	// layer2.RawBurst.VoiceSlots returns it. For AMBE+2 it is
	// vocoder.AMBESilenceSlot().
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
}

// OutputKind is the kind of an Output.
type OutputKind uint8

// The OutputKind values.
const (
	// OutputBurst is a burst to send.
	OutputBurst OutputKind = iota + 1
	// OutputEnded is the end of the transmission.
	OutputEnded
)

// String returns the name of the kind. It allocates only for a value without
// a name.
func (k OutputKind) String() string {
	switch k {
	case OutputBurst:
		return "Burst"
	case OutputEnded:
		return "Ended"
	}
	return fmt.Sprintf("OutputKind(%d)", uint8(k))
}

// Output is one result of a Transmitter. Fields that do not apply to Kind
// are zero.
type Output struct {
	// Kind is the kind of the output.
	Kind OutputKind
	// Burst is the burst to send.
	Burst layer2.RawBurst
	// Due is the time at which the burst goes on air, 60 ms after the one
	// before it.
	Due time.Duration
	// Position is the superframe position of a voice burst, and
	// VoiceBurstUnknown for a header or terminator.
	Position enums.VoiceBurstType
	// DataType is the data type of a header, PI Header or terminator. It is
	// zero, the value of enums.DataTypePIHeader, for a voice burst, so check
	// Position first.
	DataType enums.DataType
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

	vt    layer2.VoiceTransmitter
	vopts layer2.VoiceTransmitterOptions
	pi    pdu.PIHeader
	lc    pdu.FullLinkControl
	lcp   pdu.FullLinkControlPayloads

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
	taP       [4]pdu.FullLinkControlPayloads
	nTA       int
	gps       pdu.FLCGPSInfo
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
// TransmitterConfig. It returns a *ConfigError for a field out of range.
// It allocates the Transmitter and its queue, 72 bytes for each slot.
func NewTransmitter(cfg *TransmitterConfig) (*Transmitter, error) {
	if cfg == nil {
		cfg = &TransmitterConfig{}
	}
	t := &Transmitter{cc: cfg.ColorCode, source: cfg.Source, silence: cfg.Silence, headers: cfg.Headers,
		terminators: cfg.Terminators, timeout: cfg.Timeout, lead: cfg.Lead}
	if t.headers == 0 {
		t.headers = 1
	}
	if t.terminators == 0 {
		t.terminators = 1
	}
	if t.timeout == 0 {
		t.timeout = constants.TTO
	}
	queue := cfg.Queue
	if queue == 0 {
		queue = DefaultQueue
	}
	switch _, _, ok := cfg.Source.SyncPatterns(); {
	case cfg.ColorCode > 15:
		return nil, errColorCode
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
	return t, nil
}

// Reset ends any transmission without an output and empties the queue. It
// keeps the config.
func (t *Transmitter) Reset() {
	t.state, t.ended, t.timedOut, t.hasNext, t.relay, t.stopping = TransmitterIdle, false, false, false, false, false
	t.head, t.length, t.dropped, t.reason = 0, 0, 0, 0
}

// State returns the state of the Transmitter.
func (t *Transmitter) State() TransmitterState {
	return t.state
}

// Room returns the number of vocoder slots that the queue can take.
func (t *Transmitter) Room() int {
	return len(t.queue) - t.length
}

// Dropped returns the number of relayed voice bursts that the Transmitter
// dropped: voice before the LC was known, and voice that did not fit in
// the queue.
func (t *Transmitter) Dropped() int {
	return t.dropped
}

// Start begins the call c, whose first burst is due at the time at. The
// LC is c.LC when it matches the other fields of c, else it is built from
// them. opts may be nil. Start returns ErrBusy while a transmission runs,
// and a *ConfigError when c does not make an LC.
func (t *Transmitter) Start(at time.Duration, c Call, opts *CallOptions) error {
	if t.state != TransmitterIdle {
		return ErrBusy
	}
	t.relay = false
	return t.begin(at, &c, opts, false)
}

// begin sets up the call c at the time at. keep keeps the queue, which
// holds voice of c that came while the call before it ended.
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

// setLC sets the call LC from c.
func (t *Transmitter) setLC(c *Call) error {
	lc := c.LC
	if got, ok := callOf(lc, OriginHeader); !ok || !got.sameParties(c) || got.Options != c.Options || got.FID != c.FID {
		var err error
		if lc, err = pdu.NewVoiceLCBytes(c.Group, c.Target, c.Source, c.Options); err != nil {
			return errCall
		}
		lc.SetFID(c.FID)
	}
	if pdu.DecodeFullLinkControlBytesInto(&t.lc, &t.lcp, lc) != nil {
		return errCall
	}
	c.LC = lc
	return nil
}

// setExtras encodes the talker alias and GPS LCs of opts.
func (t *Transmitter) setExtras(opts *CallOptions) error {
	t.nTA = 0
	if opts.TalkerAlias != "" {
		n, err := pdu.EncodeTalkerAliasInto(&t.ta, &t.taP, opts.TalkerAlias, opts.TalkerAliasFormat)
		if err != nil {
			return errTalkerAlias
		}
		t.nTA = n
	}
	t.nExtras = t.nTA
	if opts.GPS != nil {
		t.gps = *opts.GPS
		t.gpsLC = pdu.FullLinkControl{FLCO: enums.FLCOGPSInfo, GPSInfo: &t.gps}
		if _, err := t.gpsLC.Bytes(); err != nil {
			return errGPS
		}
		t.nExtras++
	}
	return nil
}

// extra returns embedded LC i of the rotation: the talker alias LCs, then
// GPS.
func (t *Transmitter) extra(i int) *pdu.FullLinkControl {
	if i < t.nTA {
		return &t.ta[i]
	}
	return &t.gpsLC
}

// Frame queues one AMBE+2 frame. It returns ErrQueueFull when Room is 0,
// ErrTimedOut after T_TO, and ErrNotActive when no call takes voice.
func (t *Transmitter) Frame(f vocoder.AMBEFrame) error {
	return t.Slot(vocoder.EncodeAMBE72(f))
}

// Frames queues AMBE+2 frames as Frame does. On an error the frames
// before the failing one stay queued.
func (t *Transmitter) Frames(f ...vocoder.AMBEFrame) error {
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
	switch {
	case t.state == TransmitterTimedOut || t.timedOut:
		return ErrTimedOut
	case t.state != TransmitterHeader && t.state != TransmitterVoice || t.stopping:
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
// Stop is the release of the PTT, and the next Tick appends OutputEnded.
func (t *Transmitter) Stop() {
	switch t.state {
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
	switch {
	case t.state == TransmitterIdle, t.state == TransmitterTimedOut && !t.ended:
		return 0, false
	}
	return t.due - t.lead, true
}

// Tick appends the next burst when now has reached its Due time minus
// TransmitterConfig.Lead, and OutputEnded after the last terminator. A
// Tick before Deadline appends nothing. Each Tick appends at most one
// burst; a late caller calls Tick until Deadline is in the future. It
// appends to dst, and allocates only when dst has no room for MaxOutputs
// outputs.
func (t *Transmitter) Tick(now time.Duration, dst []Output) []Output {
	at, ok := t.Deadline()
	if !ok || now < at {
		return dst
	}
	if t.state == TransmitterTimedOut {
		return t.finish(dst, EndTimeout)
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
	case TransmitterIdle, TransmitterHeader, TransmitterTimedOut:
	}
	t.due += slotPeriod
	return dst
}

// ownSlots returns the number of queued slots of the current call. The
// queue can also hold voice of the relayed call that comes next.
func (t *Transmitter) ownSlots() int {
	if t.hasNext {
		return t.oldSlots
	}
	return t.length
}

// dropOwn drops the queued slots of the current call.
func (t *Transmitter) dropOwn() {
	n := t.ownSlots()
	t.head, t.length = (t.head+n)%len(t.queue), t.length-n
	t.oldSlots = 0
}

// header fills o with the next Voice LC Header or the PI Header.
func (t *Transmitter) header(o *Output) {
	switch {
	case t.sent >= t.headers && t.piPending() && t.vopts.PIHeader != nil:
		o.Burst, _ = t.vt.PIHeader()
		o.DataType = enums.DataTypePIHeader
		t.piSent = true
	case t.sent >= t.headers && t.piPending():
		o.Burst, o.DataType = t.relayPI, enums.DataTypePIHeader
		t.piSent = true
	default:
		o.Burst, o.DataType = t.vt.Header(), enums.DataTypeVoiceLCHeader
	}
	t.sent++
}

// headersDone reports whether the header group is complete, and moves to
// TransmitterVoice when it is. A relayed call whose voice has not come yet
// sends more Voice LC Headers, up to MaxHeaders, as the source of the call
// does.
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

// piPending reports whether a PI Header waits to be sent.
func (t *Transmitter) piPending() bool {
	return (t.vopts.PIHeader != nil || t.hasRelayPI) && !t.piSent
}

// voice fills o with the next voice burst, from the queue or silence.
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
	o.Position, _ = enums.VoiceBurstAt(t.pos)
	t.pos = (t.pos + 1) % layer2.VoiceBurstsPerSuperframe
	if !t.timedOut && t.timeout >= 0 && o.Due+slotPeriod-t.start >= t.timeout {
		t.ended = t.state == TransmitterEnding
		t.state, t.timedOut = TransmitterEnding, true
	}
}

// startSuperframe sets the embedded LC of the superframe that starts:
// relayed fragments, else every other superframe a talker alias or GPS
// LC, else the call LC.
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

// terminator fills o with a Terminator with LC, and appends OutputEnded
// after the last one.
func (t *Transmitter) terminator(o *Output, out []Output) []Output {
	o.Burst, o.DataType = t.vt.Terminator(), enums.DataTypeTerminatorWithLC
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

// finish appends OutputEnded with reason, and starts a relayed call that
// waited.
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
// a call that is not ending runs.
func (t *Transmitter) Relay(now time.Duration, e *Event) error {
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

// recolor returns the PI Header burst b with the color code and SYNC of the
// Transmitter.
func (t *Transmitter) recolor(b *layer2.RawBurst) layer2.RawBurst {
	r := *b
	r.SetSlotType(pdu.SlotType{ColorCode: t.cc, DataType: enums.DataTypePIHeader})
	_, data, _ := t.source.SyncPatterns()
	_ = r.SetSync(data)
	return r
}

// relayStart starts the relayed call c, or keeps it for the end of a call
// that is ending.
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
	case TransmitterTimedOut:
	}
	return ErrBusy
}

// relayVoice queues the vocoder slots of a relayed voice event, after the
// fills for the bursts lost before it.
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

// collect keeps the embedded fragments of bursts B to E of a received
// superframe, and carries them to the next superframe when they decode
// and are not a call LC other than the one sent, so that a call whose
// addresses the consumer changed keeps them.
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
	lc, res := layer2.DecodeEmbeddedLC(t.frags, false)
	if res.Uncorrectable {
		return
	}
	if _, voiceLC := callOf(pdu.LCBytes(lc), OriginEmbedded); !voiceLC || pdu.LCBytes(lc) == t.call.LC {
		t.carry, t.hasCarry = t.frags, true
	}
}

// appendOutput appends an Output of kind k due at due to out and returns
// the extended slice and the new Output. It grows out only when out lacks
// the capacity. It is not inlined, so that the growth stays in a function
// whose name tells that it appends.
//
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
