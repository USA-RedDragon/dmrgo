// Package csbk runs the CSBK services of one DMR logical channel: a request
// that waits for its answer, the answers of a radio to the requests that
// address it, the Pre_CSBKs that announce a CSBK or data, the wakeup of a
// BS outbound, and the TDMA direct mode wide area timing. (ETSI TS 102
// 361-1 clause 5.2.2, ETSI TS 102 361-2 clauses 5.1.1.1, 5.1.2, 5.2.2 and
// 6.2)
//
// Each machine is a state machine without I/O. Each method takes the time
// now, on a monotonic clock that the caller picks, and appends outputs to a
// slice of the caller. A slice with capacity MaxOutputs never grows, and
// after the constructor no method allocates. Deadline tells when a machine
// needs a Tick, and each Tick emits at most one burst, 60 ms after the
// burst before it. now is the start of the received burst; Due is the
// start of the burst to send. A Responder answers in a slot of the
// logical channel of the request, a TDMA frame or more after its start: the
// next but one slot of a direct channel, and on the aligned channel timing
// of a BS the inbound slot of the channel, which starts with its outbound
// slot (ETSI TS 102 361-1 clauses 5.1.1 and 5.2.1.7, Table 5.1). On the
// offset channel timing of a BS, ResponderConfig.Offset, the inbound slot
// of the channel starts half a TDMA frame after its outbound slot. Its
// first slot after the request starts 30 ms after it, which leaves no time
// to decode the request, so the answer goes in the slot after that, 90 ms
// after the request. ETSI gives no answer delay; this is the choice of
// dmrgo.
//
//   - Requester sends a CSBK, after Pre_CSBKs when asked, with the channel
//     access of the request: polite or impolite, the idle search T_IdleSrch
//     and the random holdoff T_Holdoff. It waits T_AckWait for the answer,
//     sends again up to N_CSBKRetry times, and reports the answer, a
//     NACK_Rsp, or why there is none. It knows the OACSU presence check
//     (UU_V_Req and UU_Ans_Rsp) and the Motorola radio check and call
//     alert (FID 0x10), and takes other CSBKs with a Match.
//   - Responder answers the CSBKs that address its radio: UU_Ans_Rsp to
//     UU_V_Req, the Motorola radio check answer and call alert
//     acknowledgement, and the Feature Not Supported NACK_Rsp to a
//     standard CSBK that it does not support.
//   - Preambles follows Pre_CSBKs and reports the CSBK or data that they
//     announce, joined with the preambles, or that it did not come.
//   - Wakeup sends BS_Dwn_Act until the BS outbound starts, up to N_Wakeup
//     times.
//   - Timing follows, elects and propagates the TDMA direct mode timing
//     leader with CT_CSBKs, and tells the caller when to take the slot
//     timing of a CT_CSBK.
//
// The machines take a burst with a layer2.Hint, decode it, and drop a CSBK
// whose CRC fails. A monitor that feeds a burst to several machines, with
// voice.Receiver and data.Receiver, decodes it once with
// layer2.Burst.DecodeFromBytesWith and calls Decoded on each, as the
// example of voice.Receiver.Decoded shows. A Requester and a Responder
// take the state of the channel from Channel; until the first report the
// channel counts as idle, as it is for a network transport.
//
// The Motorola radio check and call alert have no public spec. Their
// layouts, and the timing of their answers, come from captures of an
// AnyTone radio through a Motorola XPR 8400 repeater.
package csbk
