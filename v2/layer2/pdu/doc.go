// Package pdu decodes the layer 2 PDUs of ETSI TS 102 361 from their
// information bits into structs and encodes the structs back into bits: CSBK,
// MBC, Full LC, Short LC, data headers, PI header, slot type, EMB and the
// data blocks.
//
// DecodeX decodes a PDU and X.Encode encodes it. cmd/dmrgen generates most of
// these functions from the dmr struct tags. A CSBK, MBC header, Full LC,
// Short LC or data header keeps its payload only as bits in RawPayload, and
// its opcode or format selects the layout of the bits. AsX decodes
// RawPayload as the payload X and reports false when the opcode selects
// another payload. SetX encodes the payload X into RawPayload and sets the
// opcode with it. A caller that changes Opcode, FID, FLCO or SLCO by hand
// changes the layout that the same bits are read with. The payloads have no DecodeX
// or Encode of their own. No decode allocates. (ETSI TS 102 361-1 clause 9)
//
// # Bit order
//
// DecodeX takes the bits of a PDU packed in bytes, and Encode returns them
// in the same form: bit 0 of the PDU, the first bit on air, is the most
// significant bit of byte 0, so a CSBK is a [12]byte and a Short LC of 36
// bits is a [5]byte. When the number of bits is not a multiple of 8,
// DecodeX ignores the low bits of the last byte and Encode sets them to 0.
// Encode returns a *dmrgo.FieldRangeError when a field value does not fit its
// bits, or when a padding bit of a raw field, such as MBCContinuation.Data,
// is set.
//
// # Setters
//
// A setter returns an error only when its argument can hold a value that the
// PDU cannot carry, and then changes nothing. LCBytes.SetFID takes a full
// octet and returns nothing. The errors are:
//   - a *dmrgo.FieldRangeError when a value does not fit its field, such as an
//     FLCO over 6 bits for LCBytes.SetFLCO, a field of the payload of a SetX
//     method, or the AppendedBlocks 0 of DataHeader.SetRawData;
//   - ErrWrongOpcode when a SetX method that takes the opcode gets one that
//     does not select X;
//   - ErrNoAddresses and ErrNoServiceOptions when the PDU has no such field.
//
// # FEC results
//
// DecodeX returns only the PDU. A PDU whose decode corrects or checks
// errors, with FEC such as Golay, QR, Hamming or RS(12,9) or with a CRC,
// has an FEC field that reports the result. A PDU without such a code, such
// as a CSBK payload or a data block, has no FEC field. Decoding never fails
// on an uncorrectable result: the fields are set from the received bits and
// FEC.Uncorrectable is set. FEC.Err returns fec.ErrUncorrectable for a
// caller that wants an error. Functions that decode a bare value, such as
// DecodeEMBCodeword, return the fec.Result with it. SetX and the other
// setters keep FEC, so a decoded PDU edited with them still reports the
// result of its decode.
//
// # CSBK and MBC payloads
//
// The FID redefines the CSBKO (ETSI TS 102 361-1 Table 9.21), so the As
// methods of CSBK and MBCHeader apply only to the standard FID, except the
// Motorola ones below. The payloads of other FIDs and of opcodes without a
// layout are read from RawPayload.
//
// Opcode 0x38 is BS_Dwn_Act in Tier II and TD_GRANT_MI in Tier III, and the
// CSBK does not say which. Read it with CSBK.AsBSOutboundActivation or
// CSBK.AsTalkgroupDataGrantMultiItem, and set it with
// CSBK.SetBSOutboundActivation or CSBK.SetTalkgroupDataGrantMultiItem. When
// the CSBK comes from a burst, CSBK.Hint0x38 gives the likely meaning, and
// the As method of the other PDU then reports false. An MBC header with
// opcode 0x38 is always TD_GRANT_MI (ETSI TS 102 361-4 Table 7.15).
//
// Read the radio check, call alert and call alert acknowledgement of the
// Motorola feature set (FID 0x10) with CSBK.AsMotorolaRadioCheck,
// CSBK.AsMotorolaCallAlert and CSBK.AsMotorolaCallAlertAck, and set them
// with the matching Set methods.
//
// The header of a grant, C_MOVE or C_BCAST MBC carries the CSBK payload of
// its opcode (ETSI TS 102 361-4 clause 7.1.1.1). DecodeMBCContinuation
// decodes every continuation block in the same way, because the layout
// depends on the header. Pick the layout from MBCHeader.Opcode with
// MBCContinuation.CGAP, MVAP, BCAP or VNAP. A CG_AP or MV_AP block
// follows a header whose PhysicalChannel is 0xFFF. The last-block CRC covers
// every continuation block (ETSI TS 102 361-1 clause 7.4.0), so the FEC
// field of one decoded block applies only when the MBC has one continuation
// block. Use SetMBCLastBlockCRC and MBCLastBlockCRCOK with all continuation
// blocks in order.
package pdu
