// Package bs is the base station side of a DMR Tier II channel: a
// repeater or hotspot with two TDMA slots and a shared CACH, or the slot
// arbiter of a network server that has no RF (ETSI TS 102 361-1 clause 5.2
// and Annex G, ETSI TS 102 361-2 clause 5.1.1, ETSI TS 102 361-3 clause
// 5.4.3.3).
//
// A Repeater is a state machine without I/O. Each method takes the time
// now, on a monotonic clock that the caller picks, and appends outputs to a
// slice of the caller. A slice with capacity MaxOutputs never grows, and
// after New no method allocates. Deadline tells when the Repeater needs a
// Tick.
//
// The Repeater checks the color code of RF bursts, gives each slot to one
// source at a time, and refuses or queues the others. It regenerates every
// burst that it repeats: voice through a voice.Receiver and a
// voice.Transmitter in relay mode, with new FEC, EMB, embedded LC and the
// local color code, and data, CSBK and header bursts from their corrected
// payload. After a voice call it holds the slot for the call hang time and
// sends Terminators with the call LC, and after confirmed data it sends
// TD_LC for the data hang time. In RF mode it makes one outbound burst
// every 30 ms: the repeated traffic, hang time bursts or Idle bursts, and
// the CACH with the TC bit, the AT bit and the Short LC (Activity Update or
// Null). It wakes up on BS_Dwn_Act or any valid burst and hibernates after
// T_MSInactiv without activity.
//
// CACH makes the CACH alone, for a consumer with its own slot logic.
package bs
