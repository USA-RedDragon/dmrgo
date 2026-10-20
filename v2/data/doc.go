// Package data receives and sends the data packets of one DMR logical
// channel: unconfirmed and confirmed packets at rate 1/2, 3/4 and 1, short
// data, UDT packets and the response packets, with the preamble CSBKs
// before them (ETSI TS 102 361-1 clause 8.2, ETSI TS 102 361-3 clauses 5
// and 6).
//
// The Receiver is a state machine without I/O. Each method takes the time
// now, on a monotonic clock that the caller picks, and appends events to a
// slice of the caller. A slice with capacity MaxEvents never grows, and
// after NewReceiver no method allocates. Deadline tells when the Receiver
// needs a Tick.
//
// The Receiver starts a data call at a preamble CSBK or a header, and ends
// it after a time without bursts, at a voice or idle burst, a transport
// end flag or the loss of the signal. It checks the header CRC, the CRC-9
// of each confirmed block and the message CRC, and delivers each complete
// packet. A block that does not arrive counts as lost: from the transport
// sequence number when the Hint has one, else from the time, one block
// per 60 ms slot. So the last block of an attempt that is lost on air
// still ends the attempt. With ReceiverConfig.Respond, it answers a
// confirmed packet to its own address with an ACK, a SACK for the blocks
// that failed, or a NACK, and it drops a packet that repeats the one
// before (the same N(S) and message CRC). A response heard on the channel
// is an event too, so a bridge can forward it.
package data
