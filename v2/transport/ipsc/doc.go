// Package ipsc encodes and decodes Motorola IP Site Connect (IPSC) packets
// as MOTOTRBO repeaters send them over UDP: the peer control packets
// (registration, keepalive, peer list), the voice and data packets with
// their RTP header, and the packets it does not decode further. It does no
// IO and keeps no state.
//
// User packets carry decoded payloads, not the 264-bit burst: three AMBE+2
// frames without FEC, or the BPTC or rate 3/4 payload of a data burst.
// User.RawBurst rebuilds the burst and User.SetRawBurst takes one apart, and
// User.Hint gives the layer2.Hint for the service machines. The Hint marks
// voice bursts A and E only; a layer2.PositionTracker places B, C, D and F.
//
// Decode functions never allocate and never panic. They return the static
// errors ErrShort, ErrLength and ErrCommand, which errors.Is matches. The
// packets are without the 10-octet HMAC-SHA1 trailer of authenticated IPSC,
// which package auth signs and checks.
//
// (DMRlink ipsc_const.py; ipsc2hbrp and DMRHub; captures of a Motorola XPR
// 8400, 2026-10-15 to 2026-10-20)
package ipsc
