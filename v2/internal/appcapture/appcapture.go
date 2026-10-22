// Package appcapture holds application data captured from real radios, for
// the tests and examples of the application protocol packages. Each value
// is the user data of one DMR data packet, after the message CRC. The
// radio is a BTECH DA-7X2 with AnyTone D890UV firmware, ID 3191868, sending
// to 3140598 through an XPR 8400 repeater, captured on 2026-10-16 unless a
// value says otherwise.
package appcapture

// TMS is the AnyTone M-SMS text "MOTO TEST" as relayed over IPSC: an IPv4
// packet from 12.48.180.60 to 12.47.235.246 with a UDP datagram from port
// 4007 to 4007 that holds a MOTOTRBO text message.
const TMS = "4500003a000d00004011c2140c30b43c0c2febf60fa70fa70026509e001ca00081040d000a004d004f0054004f00200054004500530054000000"

// DMRText is the AnyTone "DMR Standard" text "DMR TEST" as relayed over
// IPSC: an IPv4 packet with a UDP datagram from port 5016 to 5016 whose
// payload is 00 0d 00 0a, the text in UTF-16LE and a 16-bit NUL.
const DMRText = "45000032000c00000111011e0c30b43c0c2febf613981398001edcd6000d000a44004d005200200054004500530054000000"

// HSMS is the payload of the AnyTone H-SMS text "HYT TEST" received on air:
// a defined data packet (DD_HEAD, SAP short data, DD format 1) that holds a
// 16-bit field (here 45 00), the text in UTF-16LE and a 16-bit NUL.
const HSMS = "4500480059005400200054004500530054000000"

// GPS is an AnyTone position report received on air on 2026-10-20: a
// compressed UDP/IPv4 header with ports 10110 in the extended headers,
// then an NMEA RMC sentence of the default position of the radio.
const GPS = "0001000000277e277e244750524d432c3230303033372e3030302c412c333431322e373330302c4e2c31303835302e303030302c452c302e30302c3132312e30302c3032313232352c2c2c412a36300d0a"

// D878Text is a "DMR Standard" text "TEST" from an AnyTone radio, from the
// hbnet wiki page SMS-GPS-Data-Formats (github.com/kf7eel/hbnet/wiki,
// commit 920da328). The wiki names no model; the hblink3 CHANGELOG of the
// same author (github.com/kf7eel/hblink3, commit 51e2fb60) says that a D878
// was used. It is without the pad octets and the CRC: an IPv4 packet
// with a UDP datagram from port 5016 to 5016 whose payload is 00 0d 00 0a
// and the text in UTF-16LE, with no NUL. Its IPv4 and UDP checksums match.
const D878Text = "450000280005000001115f4f0c301eb70c00238b1398139800143e0c000d000a5400450053005400"

// The BTECH6X2Pro values are the three "DMR Standard" texts of a BTECH 6X2
// Pro, from KN4OQW/waypoint commit 6358a67a,
// internal/dmrdata/testdata/capture-radio-etsi.txt, as packet.Assembler
// reassembles its bursts, without the pad octets: IPv4 packets with UDP
// datagrams from port 5016 to 5016 whose payload is 00 0d 00 0a and the
// text in UTF-16LE, with no NUL. The UDP length ends at the last character.
const (
	BTECH6X2ProTest   = "4500002800000000011194110c3086aa0c3086aa13981398001472c9000d000a5400450053005400"
	BTECH6X2ProHello1 = "4500002a000100000111940e0c3086aa0c3086aa1398139800163ec5000d000a480045004c004c004f00"
	BTECH6X2ProHello2 = "4500002a000200000111940d0c3086aa0c3086aa1398139800163ec5000d000a480045004c004c004f00"
)

// HyteraIPText is a text "Test Message Hytera to DSDFME" from a Hytera
// radio, from the unencrypted capture in dsd-fme issue 372
// (github.com/lwvmobile/dsd-fme/issues/372), without the 5 pad octets and
// the CRC: a compressed UDP/IPv4 header (SAP 3) with ports 5016, then the
// text in UTF-16BE with no lead and no NUL.
const HyteraIPText = "100002010100540065007300740020004d006500730073006100670065002000480079007400650072006100200074006f00200044005300440046004d0045"
