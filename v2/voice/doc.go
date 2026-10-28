// Package voice turns the bursts of one DMR logical channel into voice call
// events: the start of a call and where its identity came from, each voice
// burst with its superframe position and per-frame quality, the embedded LC
// data such as GPS and talker alias, and the end of the call with its reason.
//
// A Receiver is a state machine without I/O. Each method takes the time
// now, on a monotonic clock that the caller picks, and appends events to a
// slice of the caller. A slice with capacity MaxEvents never grows, and
// after NewReceiver no method allocates. Deadline gives the time when the
// Receiver needs a Tick.
//
// The Receiver places voice bursts in the superframe from the strongest
// evidence it has: the position from the transport (layer2.Hint), the voice
// SYNC of burst A, the transport sequence number, the LCSS of the EMB, and
// last the time since the previous burst, in 60 ms steps. It reports lost and
// duplicate bursts, gets the call from the Voice LC Header or, on late
// entry, from the embedded LC of bursts B to E, and ends the call on a
// terminator, a data or idle burst, a transport end flag, the loss of the
// signal, or a time without bursts. It does not check whether a call is for
// the user: the consumer filters on Call.Target.
// (ETSI TS 102 361-1 clause 5.1.2, Annex G, ETSI TS 102 361-2 clause 5.2)
//
// A burst without a SYNC is voice only by elimination, so a voice burst that
// the transport does not mark (a zero Hint.Kind, no Hint.VoiceBurst and no
// Hint.Seq, as from an RF demodulator) must pass a sync check. The Receiver
// scores the evidence that the burst is voice and not noise as a
// log-likelihood ratio, the larger of two: the distance of the SYNC field to
// a voice SYNC, lowered where the timing of the call puts another burst than
// A; or the distance of the EMB to the codewords of the color code of the
// call (ReceiverConfig.ColorCode, else that of the Voice LC Header, else the
// one that most error-free EMBs give, at least two), and, for an EMB within
// 5 bits of one, the distance of the embedded data to the fragments of the
// call LC, to a null fragment, or to the fragments of the three superframes
// before. To that it adds the bit errors that the Golay codes of the three
// AMBE+2 frames correct, except in a Codec2 call (FID 0xC2), and only as a
// penalty when the SYNC, EMB and embedded data give no support. It adds the
// score to the odds that the channel carries the call, which a call start, a
// Voice LC Header or a PI Header of the call raises, and the end of a call,
// a Terminator with LC or another data burst lowers. A burst with a strong
// sum goes out at once. A doubtful one waits, with up to five after it, for
// a burst whose evidence makes the sum strong, or for a Terminator with LC:
// then the waiting bursts go out first, late by 60 ms for each burst after
// them. Waiting bursts are dropped when the sum falls, when no burst comes
// for 360 ms, on Lost, on a new transport stream, and when the call ends.
// Before a late entry only those go out that have strong evidence of their
// own, or a SYNC or an EMB that fits the place and color code of the burst
// that starts it. A burst identical to the one just before it never counts
// as strong. Without a call the odds are low, so a late entry needs a much
// higher score than a burst in a call. A burst that the check drops lowers
// the odds and is counted in the Lost of the next voice burst, and 13 in a
// row end the call with EndSyncLost. The embedded LC of a superframe that
// does not decode alone is decoded from the bitwise majority of it and the
// two superframes before. (ETSI TS 102 361-1 Annex G.1.1)
//
// A Transmitter makes the bursts of one call, as the source of the call or
// as the relay of the events of a Receiver. A source call follows the
// channel access policy and the transmit admit criteria, starts with the
// channel authorization when it begins in hang time, and ends on a Tier 2
// RC cease command. An Interrupter sends that command to the radio that
// talks. (ETSI TS 102 361-1 clause 5.2, ETSI TS 102 361-2 clauses 5.1.1.6
// and 6.3)
//
// A Jitter goes in front of a Receiver that takes a network stream. It puts
// the packets back in the order of their sequence numbers and plays them a
// fixed delay after they were sent, as the Jitter of MMDVMHost does, so
// that network jitter adds no fills and reordering loses no voice.
package voice
