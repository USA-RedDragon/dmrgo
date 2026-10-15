// Package vocoder converts between PCM and the 72-bit vocoder slots of a DMR
// voice burst. It defines the Codec interface and a name-based codec
// registry, the AMBE+2 frame FEC (EncodeAMBE72, DecodeAMBE72) and the IPSC
// AMBE payload packing. It contains no AMBE+2 codec.
//
// The Uncorrectable flag of DecodeAMBE72 reflects only the overall parity
// bit of section A. mbelib, for example, repeats the last frame when more
// than 3 data bits of sections A and B were corrected, and it does not check
// the section A parity bit.
package vocoder
