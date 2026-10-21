// Package sms reads and builds the text messages of DMR radios in every
// format that they use: MOTOTRBO TMS, the ETSI IP text, the DMR Standard
// text of AnyTone radios, ETSI defined data and the H-SMS text of AnyTone
// radios. Decode finds the format of a received data packet and returns
// the text as UTF-8; AppendPacket builds the user data and data header of
// a text in a chosen format. It does no I/O and does not allocate except
// to grow a slice of the caller. (ETSI TS 102 361-3 clauses 5.7.1 and 6.1)
package sms
