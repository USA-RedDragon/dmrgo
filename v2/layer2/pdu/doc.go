// Package pdu decodes the layer 2 PDUs of ETSI TS 102 361 from their
// information bits into structs and encodes the structs back into bits: CSBK,
// MBC, Full LC, Short LC, data headers, PI header, slot type, EMB and the data
// blocks (ETSI TS 102 361-1 clause 9).
//
// DecodeX decodes a PDU and X.Encode encodes it. cmd/dmrgen generates most of
// these functions from the dmr struct tags. The payloads of a CSBK, MBC, Full
// LC, Short LC or data header decode with their parent, which sets the
// payload field of its opcode, so they have no DecodeX of their own.
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
// DecodeEMBCodeword, return the fec.Result with it.
//
// # CSBK and MBC payloads
//
// The FID redefines the CSBKO (ETSI TS 102 361-1 Table 9.21), so the payload
// fields of CSBK and MBCHeader apply only to the standard FID. The payloads
// of other FIDs and of opcodes without a layout stay in RawPayload. Encode
// returns an error when more than one payload field is set, or when Opcode
// and FID do not select the payload field that is set.
//
// Opcode 0x38 is BS_Dwn_Act in Tier II and TD_GRANT_MI in Tier III, and the
// CSBK does not say which, so its payload stays in RawPayload. Read it with
// CSBK.AsBSOutboundActivation or CSBK.AsTalkgroupDataGrantMultiItem, and set
// it with CSBK.SetBSOutboundActivation or CSBK.SetTalkgroupDataGrantMultiItem.
// When the CSBK comes from a burst, CSBK.Hint0x38 gives the likely meaning.
//
// The header of a grant, C_MOVE or C_BCAST MBC carries the CSBK payload of
// its opcode (ETSI TS 102 361-4 clause 7.1.1.1). DecodeMBCContinuation
// decodes every continuation block in the same way, because the layout
// depends on the header. Pick the layout from MBCHeader.Opcode with
// MBCContinuation.AsCGAP, AsMVAP, AsBCAP or AsVNAP. A CG_AP or MV_AP block
// follows a header whose PhysicalChannel is 0xFFF. The last-block CRC covers
// every continuation block (ETSI TS 102 361-1 clause 7.4.0), so the FEC
// field of one decoded block applies only when the MBC has one continuation
// block. Use SetMBCLastBlockCRC and MBCLastBlockCRCOK with all continuation
// blocks in order.
package pdu
