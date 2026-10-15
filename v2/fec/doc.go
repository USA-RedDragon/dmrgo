// Package fec reports the outcome of the FEC and CRC checks of a DMR decode
// as a Result, and accumulates Results into a bit error rate.
//
// A decoder of one codeword, PDU or burst does not return an error for bits
// that its FEC or CRC cannot correct or check. It returns the decoded value
// with a Result, whose Uncorrectable field reports the failure, so the
// caller keeps the decoded fields. A function that returns a bare value
// returns the Result with it; a PDU struct reports it in its FEC field. An
// error from such a decoder means that the input cannot be decoded at all,
// such as a reserved data type. A caller that wants an error for an
// uncorrectable result calls Result.Err, which returns ErrUncorrectable.
//
// Functions that reassemble or check a message over several bursts, such
// as the packet Assembler and layer2.VoiceReceiver, report a failed check
// as an error. Such errors wrap ErrUncorrectable, and the functions still
// return what they decoded.
package fec
