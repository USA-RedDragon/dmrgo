// Package framer finds DMR bursts in a demodulated bit stream and returns
// each one as a Burst that holds the layer2.RawBurst.
//
// A burst is 264 bits with the 48-bit SYNC or embedded signalling field at
// burst bits 108 to 155 (ETSI TS 102 361-1 clause 6.1, clause 9.1.1). In a
// BS outbound stream each burst follows a 24-bit CACH, so the bursts repeat
// every 288 bits, and the TC bit of the CACH gives the TDMA channel of the
// burst that follows it (ETSI TS 102 361-1 clause 6.3, Table 9.24).
//
// Search and Framer take the stream packed in bytes, the first bit in the
// most significant bit of byte 0, and give positions as bit indexes, so a
// burst can start at any bit of a byte. Search finds a SYNC at any bit
// offset. A Framer locks on a SYNC and then emits a burst every Period bits
// whether or not it carries a SYNC, so that voice bursts B to F are emitted
// too. A SYNC found at its expected place restarts the miss count. After
// MaxMisses bursts without one, the Framer searches again from the bit after
// the last expected SYNC.
package framer
