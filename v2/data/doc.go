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
// it after a time without bursts, at a voice or idle burst, a transport end
// flag or the loss of the signal. It checks the header CRC, the CRC-9 of
// each confirmed block and the message CRC, and delivers each complete
// packet. A block that does not arrive counts as lost: from the transport
// sequence number when the Hint has one, else from the time, one block per
// 60 ms slot. So the last block of an attempt that is lost on air still
// ends the attempt. A C_HEAD packet is confirmed, and a DD_HEAD or R_HEAD
// packet is confirmed when its Response Requested flag (A) is set, with or
// without SARQ (ETSI TS 102 361-3 clause 6.4, Table 6.5). With
// ReceiverConfig.Respond, it answers a confirmed packet to its own address
// with an ACK, a SACK for the blocks that failed, or a NACK. The sequence rule depends on the layer. A packet of the IP
// bearer service (SAP UDP/IP header compression, IP based packet data or
// ARP, in the header or in the P_HEAD after it) gets no N(S) check: each
// valid packet is delivered and gets an ACK, so a packet sent again after a
// lost response is delivered again (ETSI TS 102 361-3 clauses 5.4.1.3 and
// 5.4.2.2). For any other
// SAP, a packet with N(S) equal to VI is a duplicate that gets an ACK and
// is not delivered, a packet with N(S) equal to VI + 1 is the next one, any
// other N(S) gets a NACK out of sequence with status VI, and the first
// packet of a link or a packet with the S flag sets VI (ETSI TS 102 361-1
// clauses 8.2.2.3, 9.3.23 and 9.3.24). A response heard on the channel is
// an event too, so a bridge can forward it. The fragments of a confirmed
// message are each delivered at once, and EventReassembled follows the last
// one with the whole message (ETSI TS 102 361-1 clause 9.3.25).
//
// By default a Sender uses stop and wait: each packet asks for a response.
// With SenderConfig.Window, a Sender uses sliding window flow control for
// the IP bearer service: it sends up to 8 C_HEAD packets one after the
// other, and only the last one asks for a response (ETSI TS 102 361-3
// clause 5.4.4). A window is opt-in because a common radio, the AnyTone
// D890UV firmware, acknowledges a whole window but keeps only its last
// packet. A Receiver keeps the result of each packet of a window
// and answers at the last one for all of them: an ACK with the N(S) of the
// last packet when all are good, else a response about the first packet
// that failed, with its N(S) as the status. ETSI TS 102 361-1 Table 8.3
// gives the status of a response both as the packet it is about and as
// the last packet received correctly, so a NACK about a packet after the
// first of a window goes out as an ACK of the packet before it, which
// means the same in both readings; the NACK follows when that packet comes
// again as the first of a window. It keeps the blocks of the
// last packet only, so a SACK about an earlier packet flags all its blocks,
// and a packet found lost from a gap in N(S) gets a SACK that flags 127
// blocks. The Receiver uses go back N: after the first packet of a window
// that failed or was lost, it does not deliver the later packets of that
// window, and the Sender sends them again. So each packet is delivered
// once, with no buffer for later packets. IPv4 and UDP do not drop copies
// (RFC 1122 clauses 1.1.3 and 4.1.1), so a lost response still gives a copy, as it
// does with stop and wait.
//
// Output.ICMP gives the ICMP error message that a message that failed
// reports to the IP layer, and Event.ICMP the one that a radio sends about
// an IPv4 packet that it received (ETSI TS 102 361-3 clauses 5.2 and
// 5.4.2.1). Neither gives one about a message of a group call, which is a
// link layer broadcast (RFC 1122 clause 3.2.2).
//
// Multi-fragment reassembly is tested only against the 14-fragment example
// of ETSI TS 102 361-1 Table 8.2 and synthetic packets, as no radio at hand
// sends fragments. A BTECH DA-7X2 (AnyTone D890UV firmware) sends confirmed
// SMS with FSN 1000 and unconfirmed GPS with FSN 0000. It sends N(S) 0 for
// every new message and sets F to 1 on a retry, against ETSI TS 102 361-1
// clauses 9.3.24 and 9.3.20. The rule for the IP bearer service does not
// depend on N(S) or F, so its SMS is delivered as the standard says.
package data
