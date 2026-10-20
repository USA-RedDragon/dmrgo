// Package voice turns the bursts of one DMR logical channel into voice call
// events: the start of a call and where its identity came from, each voice
// burst with its superframe position and per-frame quality, the embedded LC
// data such as GPS and talker alias, and the end of the call with its reason.
//
// A Receiver is a state machine without I/O. Each method takes the time
// now, on a monotonic clock that the caller picks, and appends events to a
// slice of the caller. A slice with capacity MaxEvents never grows, and
// after NewReceiver no method allocates. Deadline tells when the Receiver
// needs a Tick.
//
// The Receiver places voice bursts in the superframe from the strongest
// evidence it has: the position from the transport (layer2.Hint), the voice
// SYNC of burst A, the transport sequence number, the LCSS of the EMB, and
// last the time since the previous burst, in 60 ms steps. It reports lost and
// duplicate bursts, learns the call from the Voice LC Header or, on late
// entry, from the embedded LC of bursts B to E, and ends the call on a
// terminator, a data or idle burst, a transport end flag, the loss of the
// signal, or a time without bursts. It does not decide whether a call is for
// the user: the consumer filters on Call.Target.
// (ETSI TS 102 361-1 clause 5.1.2, Annex G, ETSI TS 102 361-2 clause 5.2)
package voice
