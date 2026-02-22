// Package constants defines timers and constants specified
// in ETSI TS 102 361-2 Annex A and §6.1.
package constants

import "time"

// ── A.1 Layer 3 Timers ──

// TAckWait is the recommended ACKWait timer value.
// An MS transmitting a CSBK waits this long for a response before retrying.
// ETSI TS 102 361-2 — Table A.1
const TAckWait = 360 * time.Millisecond

// TAckWaitSimulcast is the recommended minimum ACKWait timer for simulcast systems.
// ETSI TS 102 361-2 — Table A.1
const TAckWaitSimulcast = 2 * time.Second

// TTO is the transmit timeout timer (Tier I fixed value).
// Limits the maximum duration of a single transmission.
// ETSI TS 102 361-2 — Table A.1 / §6.1
const TTO = 180 * time.Second

// TTOMax is the maximum configurable transmit timeout for Tier II/III.
// A value of 0 disables the timeout.
// ETSI TS 102 361-2 — Table A.1
const TTOMax = 180 * time.Second

// CTRHOTMax is the maximum CT_CSBK Random HoldOff Timer value
// when leader timing is unknown.
// ETSI TS 102 361-2 — Table A.1
const CTRHOTMax = 3240 * time.Millisecond

// CTRHOTIncrement is the increment for the CT_CSBK Random HoldOff Timer.
// ETSI TS 102 361-2 — Table A.1
const CTRHOTIncrement = 60 * time.Millisecond

// NoLeader is the time an MS monitors the channel for wide-area
// timing information before transmitting a request.
// ETSI TS 102 361-2 — Table A.1
const NoLeader = 4*time.Minute + 30*time.Second

// SyncAge is the validity period of wide-area timing synchronisation info.
// ETSI TS 102 361-2 — Table A.1
const SyncAge = 10 * time.Minute

// SyncAgeIncrement (SAIncr) is the increment resolution for SyncAge.
// ETSI TS 102 361-2 — Table A.1
const SyncAgeIncrement = 500 * time.Millisecond

// SyncAgeWarning is the time without updated sync before an MS
// requests a timing update.
// ETSI TS 102 361-2 — Table A.1
const SyncAgeWarning = 9 * time.Minute

// TMSChanAuth is the recommended Channel Authorization timer for single-site.
// ETSI TS 102 361-2 — Table A.1
const TMSChanAuth = 180 * time.Millisecond

// TMSChanAuthMultiSite is the recommended Channel Authorization timer for multi-site.
// ETSI TS 102 361-2 — Table A.1
const TMSChanAuthMultiSite = 360 * time.Millisecond

// TBSChanAuthRspVoice is the Channel Authorization Response timer for voice (single/multi-site).
// ETSI TS 102 361-2 — Table A.1
const TBSChanAuthRspVoice = 720 * time.Millisecond

// TBSChanAuthRspCSBKSingle is the Channel Authorization Response timer
// for CSBK/data, single-site.
// ETSI TS 102 361-2 — Table A.1
const TBSChanAuthRspCSBKSingle = 900 * time.Millisecond

// TBSChanAuthRspCSBKMulti is the Channel Authorization Response timer
// for CSBK/data, multi-site.
// ETSI TS 102 361-2 — Table A.1
const TBSChanAuthRspCSBKMulti = 1140 * time.Millisecond

// TBSChanAuthSel is the recommended Channel Authorization Selection timer (multi-site only).
// ETSI TS 102 361-2 — Table A.1
const TBSChanAuthSel = 240 * time.Millisecond

// TRCtimer is the recommended Tier 2 RC timer.
// Time an MS waits for the target MS to end its transmission
// before retrying a Reverse Channel command.
// ETSI TS 102 361-2 — Table A.1 / §6.3
const TRCtimer = 600 * time.Millisecond

// ── A.2 Layer 3 Constants ──

// NCSBKRetry is the recommended CSBK retry limit.
// ETSI TS 102 361-2 — Table A.2
const NCSBKRetry = 1

// BeaconDuration is the minimum duration of a CT_CSBK_Beacon
// or CT_CSBK_Prop transmission.
// ETSI TS 102 361-2 — Table A.2
const BeaconDuration = 600 * time.Millisecond

// BeaconInterval is the time between the start of two consecutive
// CT_CSBK_Beacon transmissions by the wide-area timing leader MS.
// ETSI TS 102 361-2 — Table A.2
const BeaconInterval = 4*time.Minute + 30*time.Second

// CTDuration is the minimum duration of a CT_CSBK transmission.
// ETSI TS 102 361-2 — Table A.2
const CTDuration = 180 * time.Millisecond

// ── ETSI TS 102 361-3 Annex A: PDP (Packet Data Protocol) Timers and Constants ──

// ── A.1 Layer 2 Timers ──

// TDataTxLmt is the data transmission limit timer.
// Time duration an MS will attempt to transmit unconfirmed data
// or transmit confirmed data and receive a reply.
// ETSI TS 102 361-3 — Table A.1
const TDataTxLmt = 60 * time.Second

// TRspnsWait is the confirmed data response wait limit timer.
// Time duration an MS will wait for a confirmed header packet data response.
// ETSI TS 102 361-3 — Table A.1
const TRspnsWait = 180 * time.Millisecond

// TRspnsWaitSimulcast is the recommended minimum response wait timer for simulcast systems.
// ETSI TS 102 361-3 — Table A.1
const TRspnsWaitSimulcast = 2 * time.Second

// THoldoffMax is the maximum random holdoff time.
// MS randomly generates a holdoff timer in [0, THoldoffMax] to minimise
// collisions when data messages are queued and the channel becomes idle.
// ETSI TS 102 361-3 — Table A.1
const THoldoffMax = 2 * time.Second

// TDataHngtime is the data hangtime timer.
// Time that BS transmits TD_LC PDUs to reserve the channel for
// a confirmed data response. Recommended value is 3 traffic bursts.
// ETSI TS 102 361-3 — Table A.1
const TDataHngtime = 180 * time.Millisecond

// ── A.2 Layer 2 Constants ──

// NRtryLmt is the data air interface retry limit.
// Number of times the DLL will transmit and attempt to receive
// the confirmed data response.
// ETSI TS 102 361-3 — Table A.2
const NRtryLmt = 8
