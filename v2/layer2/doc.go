// Package layer2 decodes 33-octet DMR bursts into a Burst model and encodes
// them back. It also handles the embedded LC, the CACH and Short LC, the
// Reverse Channel, and the voice superframe. The PDUs that the bursts carry
// are in package pdu. (ETSI TS 102 361-1 clause 6 and clause 9)
//
// # Bursts
//
// A RawBurst holds the 264 bits of a burst in transmit bit order (ETSI TS
// 102 361-1 Annex E). A data burst has information bits at burst bits 0 to
// 97 and 166 to 263, the slot type at bits 98 to 107 and 156 to 165, and a
// SYNC pattern or an EMB and embedded data at bits 108 to 155. A voice burst
// has its three 72-bit vocoder slots in bits 0 to 107 and 156 to 263. A
// standalone inbound RC burst has EMB(8) RC(16) SYNC(48) RC(16) EMB(8)
// around the burst center (ETSI TS 102 361-1 clause 6.4.1, Figure 6.7). The
// methods of RawBurst read or write one field and leave every other bit as
// it is. They do not classify the burst.
//
// Burst.DecodeFromBytes classifies a burst by its SYNC field. A burst
// without a SYNC pattern is a voice burst B to F, unless it is a data burst
// with an EMB in place of the SYNC (ETSI TS 102 361-1 clause 6.4.2). Such a
// data burst has a center more than 12 bits from every data SYNC pattern, an
// EMB that decodes, a slot type with at most 2 corrections and the color
// code of the EMB, a BPTC coded data type, a BPTC with at most 8
// corrections, and a CRC or RS(12,9) code that checks where the data type
// has one. Decoding is stateless per burst, so a voice burst B to F decodes
// at VoiceBurstUnknown. Its EMB and embedded data can come within the SYNC
// threshold of a SYNC pattern and then decode as that kind of burst. When
// the stream gives the position, as a network voice frame does, set
// DecodeOptions.Kind and DecodeOptions.VoiceBurst.
//
// Burst.Encode writes a voice burst with a voice SYNC pattern, the reserved
// pattern, or embedded signalling, and a data burst with its slot type and a
// data SYNC pattern. A data burst carries the bits that the Encode method of
// Data returns, coded for SlotType.DataType. Data must be a PDU type of
// package pdu. The BurstDataType of a Full LC selects its RS(12,9) mask, so
// it must match the slot type. A data burst has SyncPattern enums.SyncNone
// only when DecodeFromBytes found an EMB in place of its SYNC. To build a
// new data burst with an EMB, use BuildBurst and then RawBurst.SetEMB and
// RawBurst.SetEmbeddedData. A data burst from
// DecodeFromBytes whose Data is nil, or codes to the same bits as the
// decoded payload, goes out with the payload as it was decoded after FEC
// correction. A burst whose CRC failed can thus be forwarded. The embedded
// data field goes out as it is, because the FEC of an embedded LC spans
// bursts B to E. Voice slots go out as they are, so a slot of any codec
// passes through.
//
// # Voice
//
// A voice transmission is a Voice LC Header, then complete superframes of
// six voice bursts A to F, then a Terminator with LC (ETSI TS 102 361-1
// clause 5.1.2). Burst A carries a voice SYNC pattern. Bursts B to E carry
// the embedded Full LC in four fragments with LCSS First, Continuation,
// Continuation and Last (ETSI TS 102 361-1 clause 7.1.3). The embedded LC
// has no RS(12,9) parity: a 5-bit checksum protects it (ETSI TS 102 361-1
// clause B.3.11). Burst F carries the Null embedded message, or an outbound
// RC from a BS (ETSI TS 102 361-1 clause 6.4.2). Each voice burst carries
// three 72-bit vocoder slots. The slots are opaque: an AMBE+2 frame is a
// slot after vocoder.EncodeAMBE72, and another codec supplies its own slots.
// VoiceTransmitter builds the bursts from the slots and pads the last
// superframe with a silence slot.
//
// VoiceReceiver sets the superframe position of each voice burst and
// reassembles the embedded LC. A burst with a voice SYNC pattern is burst A.
// A burst with LCSS First is burst B and a burst with LCSS Last is burst E,
// so a lost burst A does not lose the superframe. Any other embedded
// signalling burst takes the position after the previous burst when its LCSS
// fits that position, and VoiceBurstUnknown otherwise. A burst with an
// uncorrectable EMB takes the position after the previous burst. A data
// burst or a burst with the reserved SYNC pattern ends the transmission and
// resets the receiver. A standalone RC burst leaves the receiver unchanged.
// A lost burst inside a superframe can shift the positions of the bursts
// after it until the next burst A, B or E.
//
// PositionTracker places voice bursts packet by packet from what a
// transport gives: the position or sequence number of the Hint, the voice
// SYNC and the LCSS, as voice.Receiver does. A bridge that forwards each
// packet to a transport that needs the position, such as IPSC to HomeBrew
// DMRD, uses it.
//
// # CACH and Short LC
//
// In a BS outbound stream each burst follows a 24-bit CACH (ETSI TS 102
// 361-1 clause 6.3). CACHDeinterleave puts the received bits in logical
// order: the 7-bit TACT, then a 17-bit payload that is one row of the Short
// LC BPTC matrix (ETSI TS 102 361-1 clause B.4.1, Figure B.9). Figure B.9
// numbers the bits from TX(23), sent first, to TX(0). This package packs
// the 24 bits in the order sent in 3 bytes, so TX(23) is the most
// significant bit of byte 0 and TX(0) the least significant bit of byte 2.
// ShortLCAssembler decodes a Short LC from the payloads of four CACHs with
// LCSS First, Continuation, Continuation and Last (ETSI TS 102 361-1 clause
// 7.1.4).
//
// # Errors
//
// Errors follow the rule of package fec: decoding never fails because FEC
// or a CRC cannot correct or check the bits. Burst.FEC and the FEC field of
// each PDU report that, and the decoded fields are still set, so a caller
// can inspect or forward them. An error means that the input cannot be
// decoded, such as ErrReservedDataType, or breaks a caller-given rule, such
// as ErrColorCodeMismatch. A caller that wants an error for an
// uncorrectable burst uses b.FEC.Aggregate().Err(). The errors of this
// package that report an uncorrectable result, such as
// ErrEmbeddedLCUncorrectable, wrap fec.ErrUncorrectable.
//
// A RawBurst getter that decodes a PDU, such as SlotType and EMB, reports
// the result in the FEC field of the PDU. A getter that decodes a bare
// value, such as BPTCPayload and Rate34Payload, returns a fec.Result with
// it. A getter of uncoded bits, such as EmbeddedData and VoiceSlots, returns
// the bits only.
//
// # MMDVMHost
//
// MMDVMHostAccepts reports whether MMDVMHost (g4klx/MMDVMHost 0acb758)
// uses a data burst from the network in CDMRSlot::writeNetwork, and
// MMDVMHostSeal rewrites a payload so that it passes. The checks by data
// type are:
//
//   - Voice LC Header and Terminator with LC: the RS(12,9) parity, with the
//     Data Type CRC Mask removed, must equal the parity of the 9 LC octets.
//     MMDVMHost corrects no error (CRS129::check). When the check fails on a
//     network Voice LC Header, MMDVMHost builds the LC from the DMRD header
//     fields. It builds every Terminator with LC from the header LC.
//   - CSBK, MBC Header and Data Header: the CRC-CCITT with its mask removed
//     must check. MMDVMHost also discards a BS_Dwn_Act CSBK or MBC Header,
//     which MMDVMHostAccepts does not check.
//   - MBC Continuation: MMDVMHost checks the last 16 bits of every block as
//     an unmasked CRC-CCITT of its first 80 bits. ETSI TS 102 361-1 clause
//     7.4.0 puts one CRC, over all continuation blocks, in the last block
//     only (see pdu.MBCLastBlockCRCOK). The two agree only for an MBC with a
//     single continuation block.
//   - PI Header, Rate 1/2, Rate 3/4 and Rate 1 Data: MMDVMHost checks
//     nothing and forwards the burst.
//   - Idle, Unified Single Block Data and reserved data types: MMDVMHost
//     discards the burst.
//
// MMDVMHost also discards some bursts by call state, such as data blocks
// outside a data call. This package does not check call state.
//
// MMDVMHostSeal writes the parity or CRC that pdu.Seal writes for the data
// types that have one, the PI Header included. For an MBC Continuation it
// sets octets 10 and 11 to the unmasked CRC-CCITT of octets 0 to 9. The
// CRC-9 of a confirmed data block covers its DBSN, so set it with the
// SetConfirmed methods of the pdu Rate types.
package layer2
