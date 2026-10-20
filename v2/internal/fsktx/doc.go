// Package fsktx modulates DMR 4FSK for a transmitter: the deviation signal
// that drives the modulator input of an FM radio, as int16 audio, and complex
// I/Q for a software radio. Dibits turns the bytes of a burst or a CACH into
// symbols; each modulator then takes one symbol at a time, -3, -1, +1, +3, or
// 0 for no modulation, and writes the samples of the symbol period that it
// completes.
//
// The tiers come from the Pareto front of a sweep of the modulator
// designs in CPU, RAM and an RF score, the Eb/N0 penalty at the rf
// receivers and the excess over the spectrum limits (the sweep is
// TestTxSweep):
//
//   - Tier0, MCUAudio and MCUIQ, shape by a fixed-point multiply and
//     accumulate over tables in read-only memory and fit a Cortex-M4F at 4
//     MHz in about 100 to 120 bytes of state. MCUIQ turns a 32-bit phase into I/Q
//     by a cosine table of 4096 entries.
//   - Tier1, Audio and IQ, shape in float32 over eight symbols on each
//     side of the pulse, at any output rate, and IQ turns the phase into
//     I/Q by an interpolated cosine table.
//   - Tier2 shapes in float64 over sixteen symbols on each side and IQ uses
//     float64 sine and cosine.
//
// The constructors NewAudio and NewIQ allocate the modulator; Init sets up a
// Tier0 modulator in memory of the caller. No method allocates, except the
// Append methods to grow dst. (ETSI TS 102 361-1 clause 10.2)
package fsktx
