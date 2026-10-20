// Package dvsi speaks the packet protocol of the DVSI AMBE-3000R and
// AMBE-3003 vocoder chips, and implements vocoder.Codec with such a chip for
// DMR AMBE+2 voice (3600 bit/s, 2450 bit/s of speech and 1150 bit/s of FEC).
//
// The package does no I/O of its own. The Append functions build packets
// and Parse reads them, without allocation. Codec runs the protocol over a
// Port that the caller opens: a USB serial device such as the ThumbDV or a
// DVSI USB-3000 (DVstick 30), a UART of a microcontroller, or a UDP
// connection to an AMBEserver. A Codec configures the chip in New and then
// decodes and encodes one frame per request and reply.
//
// The chip does the AMBE+2 FEC, so a slot goes to the chip as the 72
// on-air bits that layer2.RawBurst.VoiceSlots returns, and the chip returns
// the 72 bits in the same order. This is how the xlxd ambed transcoder
// uses the chips for DMR.
//
// Serial links run at 8 data bits, no parity bit, 1 stop bit, with RTS/CTS
// flow control: 460800 baud for the USB-3000 and 921600 for the USB-3003
// (DVSI USB-3000/USB-3003 User's Manual v2.6, section 2.7). The ThumbDV
// runs at 460800 baud, and its early units at 230400 (the -s option of
// AMBEserver and the half-speed option of serialDV). An AMBEserver listens
// on UDP port 2460 and carries one packet per datagram in each direction.
//
// The constants and packet formats come from the AMBE-3000R Vocoder Chip
// Users Manual v2.5 (DVSI, January 2022), chapter 6, and the AMBE-3003
// Vocoder Chip Users Manual v2.7 (DVSI, January 2026), chapter 5. Where the
// manuals disagree with themselves, the package follows their example
// packets and the open implementations xlxd ambed, AMBEserver and serialDV:
// a channel field has no data byte in a request and one status byte in a
// response, and the parity byte covers every byte after the start byte.
package dvsi
