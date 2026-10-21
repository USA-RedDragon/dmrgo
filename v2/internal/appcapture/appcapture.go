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
