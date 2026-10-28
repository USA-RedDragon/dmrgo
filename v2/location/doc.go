// Package location reads the positions that DMR radios report in data
// packets, in every format that they use: the NMEA RMC sentence of AnyTone
// radios, the MOTOTRBO LRRP reports of MOTOTRBO and Hytera radios, and the
// HDAP location protocol of Hytera radios.
// Decoder.Decode finds the format from the UDP port and returns one Fix.
// AppendPacket sends a Fix as the NMEA RMC sentence of an AnyTone radio. It
// does no I/O and does not allocate except to grow a slice of the caller.
package location
