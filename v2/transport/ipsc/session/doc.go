// Package session runs the link control of Motorola IP Site Connect
// (IPSC): the registration with the master, the peer list, the
// registration between peers, the keepalives, the expiry of silent peers
// and the HMAC-SHA1 authentication trailer, on the peer side (Peer, as a
// MOTOTRBO repeater or DMRlink runs it) and on the master side (Master).
//
// The machines do no IO. Each method takes the time now, on a monotonic
// clock that the caller picks, and the packets that the caller received
// with the address they came from, and appends outputs to a slice of the
// caller: packets to send with their address, and events. A slice with
// room for the MaxOutputs of the machine never grows, and after the
// constructor no method
// allocates or panics. Deadline tells when a machine needs a Tick. An
// OutputSend packet points into the machine and is valid until the next
// call of the machine; an OutputData packet points into the packet that
// the caller passed in.
//
// (DMRlink 1f50db2 dmrlink.py and ipsc_const.py; ipsc2hbrp; captures of a
// Motorola XPR 8400, 2026-10-15 to 2026-10-20)
package session
