// Package packet segments a payload into DMR data packets and reassembles
// received data blocks into a Message (ETSI TS 102 361-1 clause 8.2). It
// handles the header block, the rate 1/2, 3/4 or 1 data blocks, the pad
// octets, the 32-bit message CRC (ETSI TS 102 361-1 clause B.3.9), confirmed
// delivery with a DBSN and a CRC-9 per block, and the response packets
// (ETSI TS 102 361-1 clause 8.2.2.3).
//
// An Assembler takes one packet at a time. It drops a header whose CRC failed
// and returns ErrHeaderCRC, and the packet in progress stays as it was. A
// header of a first try (Full Message Flag set), other than a P_HEAD second
// header, discards any partial packet. A header of a later try (flag clear)
// with the same format, SAP and LLIDs as the confirmed or SARQ packet in
// progress, and the same N(S) for a C_HEAD, starts the retry of the blocks
// it announces. A first-try header that matches in the same way is also a
// retry, but only after all blocks of the attempt have arrived. A later-try
// header that matches nothing is dropped with ErrUnexpectedRetry while a
// packet is in progress, and otherwise starts a packet, which completes only
// when the blocks cover the whole packet and the message CRC passes. Lost
// counts a block burst that was not received, so that the attempt still
// ends. A header with no blocks to follow, such as an ACK, completes its
// Message at once. The Assembler ignores UDT and status precoded headers.
package packet
