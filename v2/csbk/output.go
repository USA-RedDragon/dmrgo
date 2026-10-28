package csbk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	idleName    = "Idle"
	accessName  = "Access"
	sendingName = "Sending"
)

// MaxOutputs is the most outputs that one call of a machine method appends.
const MaxOutputs = 2

// BurstInterval is the time between two bursts of one logical channel: one
// TDMA frame of two 30 ms slots.
const BurstInterval = 60 * time.Millisecond

// Defaults of the configs.
const (
	// DefaultIdleSearch is the idle search timer T_IdleSrch, the most that
	// ETSI TS 102 361-1 clause F.1 allows.
	DefaultIdleSearch = 540 * time.Millisecond
	// DefaultHoldoffMax is the upper end of the random holdoff T_Holdoff,
	// the recommended maximum for non-time critical CSBK ACK/NACK messages
	// (ETSI TS 102 361-1 clause F.1).
	DefaultHoldoffMax = time.Second
	// DefaultMotorolaAckWait is T_AckWait for the Motorola radio check and
	// call alert. An XPR 8400 relayed the answer of an AnyTone radio 430 ms
	// after the request went out on IPSC, which the 360 ms of
	// constants.TAckWait does not cover.
	DefaultMotorolaAckWait = 2 * time.Second
	// DefaultSyncWait is the Sync WU timer T_SyncWu, the most that ETSI TS
	// 102 361-1 clause F.1 allows.
	DefaultSyncWait = 360 * time.Millisecond
	// DefaultWakeups is the wakeup threshold N_Wakeup that ETSI TS 102
	// 361-1 clause F.2 suggests.
	DefaultWakeups = 2
	// MaxPreambles is the most Pre_CSBKs before a CSBK: the CSBK Blocks to
	// Follow of the first one counts them and the CSBK in 8 bits.
	MaxPreambles = 255
	// MaxRetries is the most retries of a request, and the most attempts of
	// a wakeup.
	MaxRetries = 255
)

// Service is the CSBK service of a request or an answer.
type Service uint8

// The Service values.
const (
	// ServiceCustom is a request that Requester.Request sends with a Match.
	ServiceCustom Service = iota
	// ServiceOACSU is the presence check of an OACSU individual call:
	// UU_V_Req, answered by UU_Ans_Rsp (ETSI TS 102 361-2 clause 5.2.2).
	ServiceOACSU
	// ServiceRadioCheck is the Motorola radio check (FID 0x10, opcode
	// 0x24), answered by a radio check with octet 3 set to 0x80.
	ServiceRadioCheck
	// ServiceCallAlert is the Motorola call alert (FID 0x10, opcode 0x1F),
	// answered by the call alert acknowledgement (opcode 0x20).
	ServiceCallAlert
	// ServiceUnsupported is a standard CSBK that addresses the radio and
	// that the Responder does not support. It answers with the Feature Not
	// Supported NACK_Rsp (ETSI TS 102 361-2 clause 5.1.2).
	ServiceUnsupported
	// ServiceWakeup is the BS outbound activation BS_Dwn_Act (ETSI TS 102
	// 361-1 clause 5.2.2.2.4).
	ServiceWakeup
	// ServiceTiming is the TDMA direct mode wide area timing of a Timing,
	// with CT_CSBK (ETSI TS 102 361-2 clause 6.2).
	ServiceTiming
)

// String returns the name of the service. It allocates only for a value
// without a name.
func (s Service) String() string {
	switch s {
	case ServiceCustom:
		return "Custom"
	case ServiceOACSU:
		return "OACSU"
	case ServiceRadioCheck:
		return "RadioCheck"
	case ServiceCallAlert:
		return "CallAlert"
	case ServiceUnsupported:
		return "Unsupported"
	case ServiceWakeup:
		return "Wakeup"
	case ServiceTiming:
		return "Timing"
	}
	return "Service(" + text.Int(int64(s)) + ")"
}

// Result is how a request or a wakeup ended.
type Result uint8

// The Result values.
const (
	// ResultAnswered is an answer that accepts: UU_Ans_Rsp with Proceed, a
	// radio check answer, a call alert acknowledgement, an answer that the
	// Match of a custom request accepts, or, for a wakeup, the BS outbound.
	ResultAnswered Result = iota + 1
	// ResultRefused is UU_Ans_Rsp with Deny.
	ResultRefused
	// ResultNACK is a NACK_Rsp for the request. Output.Reason has its
	// reason code.
	ResultNACK
	// ResultNoAnswer is no answer after the last retry, or no BS outbound
	// after the last wakeup.
	ResultNoAnswer
	// ResultDenied is a channel that stayed busy for T_IdleSrch, for a time
	// critical request or a response.
	ResultDenied
	// ResultCanceled is a request that Cancel ended.
	ResultCanceled
)

// String returns the name of the result. It allocates only for a value
// without a name.
func (r Result) String() string {
	switch r {
	case ResultAnswered:
		return "Answered"
	case ResultRefused:
		return "Refused"
	case ResultNACK:
		return "NACK"
	case ResultNoAnswer:
		return "NoAnswer"
	case ResultDenied:
		return "Denied"
	case ResultCanceled:
		return "Canceled"
	}
	return "Result(" + text.Int(int64(r)) + ")"
}

// OutputKind is the kind of an Output.
type OutputKind uint8

// The OutputKind values.
const (
	// OutputBurst is a burst to send at Due.
	OutputBurst OutputKind = iota + 1
	// OutputDone is the end of a request or a wakeup, with its Result, or a
	// response of a Responder that the channel denied.
	OutputDone
	// OutputRequest is a request for the radio of a Responder, which it
	// answers. For ServiceCallAlert the radio alerts its user.
	OutputRequest
	// OutputAnnounced is the first Pre_CSBK of a transfer.
	OutputAnnounced
	// OutputJoined is the CSBK or data header that the Pre_CSBKs announced.
	OutputJoined
	// OutputExpired is an announced transfer that did not come by Until.
	OutputExpired
	// OutputTiming reports that the caller of a Timing must set the channel
	// slot timing from the CT_CSBK in CSBK, which it passed at Due (ETSI TS 102
	// 361-2 clause 6.2.2.3).
	OutputTiming
)

// String returns the name of the kind. It allocates only for a value
// without a name.
func (k OutputKind) String() string {
	switch k {
	case OutputBurst:
		return "Burst"
	case OutputDone:
		return "Done"
	case OutputRequest:
		return "Request"
	case OutputAnnounced:
		return "Announced"
	case OutputJoined:
		return "Joined"
	case OutputExpired:
		return "Expired"
	case OutputTiming:
		return "Timing"
	}
	return "OutputKind(" + text.Int(int64(k)) + ")"
}

// Output is one result of a machine. Fields that do not apply to Kind are
// zero.
type Output struct {
	// Kind is the kind of the output.
	Kind OutputKind
	// Service is the service of OutputDone and OutputRequest.
	Service Service
	// Result is how the request ended, for OutputDone.
	Result Result
	// Reason is the reason code of a NACK_Rsp, for ResultNACK.
	Reason Reason
	// Burst is the burst to send, for OutputBurst.
	Burst layer2.RawBurst
	// Hint is what Burst is, for a transport: KindData with
	// enums.DataTypeCSBK, and the Slot of the config of the machine.
	Hint layer2.Hint
	// Due is the start of the burst to send, the time at which it goes on
	// air, 60 ms after the one before it. The answer of a Responder goes in
	// the slot of its logical channel a TDMA frame after the start of the
	// request, or a whole number of TDMA frames after that.
	Due time.Duration
	// Call is the call: of the burst, for OutputBurst; of the request, for
	// OutputDone and OutputRequest; and of the Pre_CSBKs, for
	// OutputAnnounced, OutputJoined and OutputExpired. Group reports a
	// talkgroup Target of a request or Pre_CSBK. It is zero for the CT_CSBKs
	// of a Timing, which carry no addresses.
	Call layer2.Call
	// Data reports that data follows the Pre_CSBKs, for OutputAnnounced and
	// OutputExpired, and that a data header followed them, for
	// OutputJoined. It is false for a CSBK.
	Data bool
	// Until is the end of the transfer that the Pre_CSBKs announce, for
	// OutputAnnounced and OutputExpired.
	Until time.Duration
	// CSBK is the answer, for OutputDone of a request that ended with an
	// answer; the request, for OutputRequest; the CSBK that followed the
	// Pre_CSBKs, for OutputJoined of a CSBK; and the CT_CSBK, for
	// OutputTiming.
	CSBK pdu.CSBKBytes
}

// Reason is the reason code of a NACK_Rsp (ETSI TS 102 361-2 clause 7.2.3).
type Reason uint8

// ReasonNotSupported is the reason code of the Feature Not Supported
// NACK_Rsp: the MS does not support the service or feature (ETSI TS 102
// 361-2 Table 7.13).
const ReasonNotSupported Reason = 0b00100001

// String returns the name of the reason code. It allocates only for a
// value without a name.
func (r Reason) String() string {
	if r == ReasonNotSupported {
		return "NotSupported"
	}
	return "Reason(" + text.Int(int64(r)) + ")"
}

const (
	answerProceed = 0b00100000
	answerDeny    = 0b00100001
)

func appendOutput(out []Output, k OutputKind) ([]Output, *Output) {
	out = append(out, Output{Kind: k})
	return out, &out[len(out)-1]
}
