// Package packet segments a payload into DMR data packets and reassembles
// received data blocks into a Message (ETSI TS 102 361-1 clause 8.2). It
// handles the header block, the rate 1/2, 3/4 or 1 data blocks, the pad
// octets, the 32-bit message CRC (ETSI TS 102 361-1 clause B.3.9), confirmed
// delivery with a DBSN and a CRC-9 per block, and the response packets
// (ETSI TS 102 361-1 clause 8.2.2.3).
//
// An Assembler takes one packet at a time. A header other than a P_HEAD
// second header discards any partial packet. The exception is a C_HEAD with
// the same SAP, LLIDs and N(S) as a confirmed packet that still has blocks
// missing: it starts the retry of those blocks. A header with no blocks to
// follow, such as an ACK, completes its Message at once. The Assembler
// ignores UDT and status precoded headers.
package packet
