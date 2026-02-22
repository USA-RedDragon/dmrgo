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

// ── ETSI TS 102 361-4 Annex A: Trunking Timers, Constants and Addresses ──

// ── A.1 Timers ──

// TRandTCDefault is the default random access timeout.
// MS waits up to this duration for an ack before retrying.
// ETSI TS 102 361-4 — Table A.1 (range: 2s–60s)
const TRandTCDefault = 10 * time.Second

// TNosigDefault is the default hunting timeout.
// If no signal is received for this duration, the MS hunts for a new site.
// ETSI TS 102 361-4 — Table A.1 (range: 1s–15s)
const TNosigDefault = 5 * time.Second

// TEmergTimerDefault is the default emergency timer value.
// ETSI TS 102 361-4 — Table A.1 (range: 1s–510s, 511=∞)
const TEmergTimerDefault = 120 * time.Second

// TPacketTimerDefault is the default packet data timer.
// ETSI TS 102 361-4 — Table A.1 (range: 1s–30s, 31=∞)
const TPacketTimerDefault = 10 * time.Second

// TMSMSTimerMax is the maximum MS-to-MS timer value.
// ETSI TS 102 361-4 — Table A.1 (range: 1s–4094s, 4095=∞)
const TMSMSTimerMax = 4094 * time.Second

// TMSLineTimerMax is the maximum MS-to-line timer value.
// ETSI TS 102 361-4 — Table A.1 (range: 1s–4094s, 4095=∞)
const TMSLineTimerMax = 4094 * time.Second

// TPTimerDefault is the default payload channel wait timer.
// ETSI TS 102 361-4 — Table A.1 (range: 4s–60s)
const TPTimerDefault = 10 * time.Second

// TNPTimerDefault is the default non-payload wait timer.
// ETSI TS 102 361-4 — Table A.1 (range: 2s–60s)
const TNPTimerDefault = 5 * time.Second

// TAwakeDefault is the default power save wakeup timer.
// ETSI TS 102 361-4 — Table A.1 (range: 0.1s–60s)
const TAwakeDefault = 1 * time.Second

// TVHangtimeDefault is the default voice hangtime timer.
// ETSI TS 102 361-4 — Table A.1 (range: 1s–60s)
const TVHangtimeDefault = 4 * time.Second

// TVItemDefault is the default voice item timer.
// ETSI TS 102 361-4 — Table A.1 (range: 10s–60s)
const TVItemDefault = 30 * time.Second

// TVInactiveDefault is the default voice inactive timer.
// ETSI TS 102 361-4 — Table A.1 (range: 0s–20s)
const TVInactiveDefault = 5 * time.Second

// TDInactiveDefault is the default data inactive timer.
// ETSI TS 102 361-4 — Table A.1 (range: 0s–20s)
const TDInactiveDefault = 5 * time.Second

// TDItemDefault is the default data item timer.
// ETSI TS 102 361-4 — Table A.1 (range: 1s–60s)
const TDItemDefault = 30 * time.Second

// TDHangtimeDefault is the default data hangtime timer.
// ETSI TS 102 361-4 — Table A.1 (range: 1s–60s)
const TDHangtimeDefault = 4 * time.Second

// TAnswerCallDefault is the default answer call timer.
// ETSI TS 102 361-4 — Table A.1 (range: 2s–60s)
const TAnswerCallDefault = 15 * time.Second

// TPendingDefault is the default pending timer.
// ETSI TS 102 361-4 — Table A.1 (range: 2s–60s)
const TPendingDefault = 5 * time.Second

// TDeregDefault is the default deregistration timer.
// ETSI TS 102 361-4 — Table A.1 (range: 0.2s–2s)
const TDeregDefault = 500 * time.Millisecond

// TBSInactiveDefault is the default BS inactive timer.
// ETSI TS 102 361-4 — Table A.1 (range: 1s–300s)
const TBSInactiveDefault = 60 * time.Second

// TDENREGDefault is the default deny registration timer.
// ETSI TS 102 361-4 — Table A.1 (0 or 10s–10000s, step 10)
const TDENREGDefault = 60 * time.Second

// TALSDefault is the default ALS timer.
// ETSI TS 102 361-4 — Table A.1 (range: 10s–300s)
const TALSDefault = 60 * time.Second

// TALSEDefault is the default extended ALS timer.
// ETSI TS 102 361-4 — Table A.1 (range: 10s–14400s)
const TALSEDefault = 300 * time.Second

// TALSRequestLifeSpanDefault is the default ALS request lifespan.
// ETSI TS 102 361-4 — Table A.1 (range: 1s–10s)
const TALSRequestLifeSpanDefault = 5 * time.Second

// TALSRetransmitDelayDefault is the default ALS retransmit delay.
// ETSI TS 102 361-4 — Table A.1 (range: 0s–5s)
const TALSRetransmitDelayDefault = 1 * time.Second

// ── A.6 Constants ──

// NDefaultNW is the default NRand_Wait value for random access.
// ETSI TS 102 361-4 — Table A.6
const NDefaultNW = 5

// NRandNR is the NRand_Wait value for normal registration.
// ETSI TS 102 361-4 — Table A.6
const NRandNR = 6

// NRandNE is the NRand_Wait value for emergency registration.
// ETSI TS 102 361-4 — Table A.6
const NRandNE = 10

// NMaint is the maintenance retry count.
// ETSI TS 102 361-4 — Table A.6
const NMaint = 4

// NmaxCh is the maximum number of channels.
// ETSI TS 102 361-4 — Table A.6
const NmaxCh = 50

// ChPref is the channel preference.
// ETSI TS 102 361-4 — Table A.6
const ChPref = 50

// NSYSerrDefault is the default system error threshold.
// ETSI TS 102 361-4 — Table A.6 (range: 1–3)
const NSYSerrDefault = 2

// DMRLADefault is the default location area count.
// ETSI TS 102 361-4 — Table A.6 (range: 1–10)
const DMRLADefault = 1

// VoteBlkDefault is the default vote block count.
// ETSI TS 102 361-4 — Table A.6 (range: 2–10)
const VoteBlkDefault = 3

// ── A.8 Well-known Gateway Addresses ──

const (
	// GatewayPSTNI is the PSTN gateway inbound address.
	// ETSI TS 102 361-4 — Table A.8
	GatewayPSTNI = 0xFFFEC0
	// GatewayPABXI is the PABX gateway inbound address.
	GatewayPABXI = 0xFFFEC1
	// GatewayLINEI is the line gateway inbound address.
	GatewayLINEI = 0xFFFEC2
	// GatewayIPI is the IP gateway inbound address.
	GatewayIPI = 0xFFFEC3
	// GatewaySUPLI is the SUPL gateway inbound address.
	GatewaySUPLI = 0xFFFEC4
	// GatewaySDMI is the SDM gateway inbound address.
	GatewaySDMI = 0xFFFEC5
	// GatewayREGI is the registration gateway address.
	GatewayREGI = 0xFFFEC6
	// GatewayMSI is the MS gateway address.
	GatewayMSI = 0xFFFEC7
	// GatewayDIVERTI is the divert gateway address.
	GatewayDIVERTI = 0xFFFEC9
	// GatewayTSI is the TS gateway address.
	GatewayTSI = 0xFFFECA
	// GatewayDISPATI is the dispatch gateway address.
	GatewayDISPATI = 0xFFFECB
	// GatewaySTUNI is the stun gateway address.
	GatewaySTUNI = 0xFFFECC
	// GatewayAUTHI is the authentication gateway address.
	GatewayAUTHI = 0xFFFECD
	// GatewayGPI is the general purpose gateway address.
	GatewayGPI = 0xFFFECE
	// GatewayKILLI is the kill gateway address.
	GatewayKILLI = 0xFFFECF
	// GatewayPSTNDI is the PSTN divert gateway address.
	GatewayPSTNDI = 0xFFFED0
	// GatewayPABXDI is the PABX divert gateway address.
	GatewayPABXDI = 0xFFFED1
	// GatewayLINEDI is the line divert gateway address.
	GatewayLINEDI = 0xFFFED2
	// GatewayDISPATDI is the dispatch divert gateway address.
	GatewayDISPATDI = 0xFFFED3
	// GatewayALLMSI is the all MS address.
	GatewayALLMSI = 0xFFFED4
	// GatewayIPDI is the IP divert gateway address.
	GatewayIPDI = 0xFFFED5
	// GatewayDGNAI is the DGNA gateway address.
	GatewayDGNAI = 0xFFFED6
	// GatewayTATTSI is the TATTS gateway address.
	GatewayTATTSI = 0xFFFED7
	// AllMSIDLocal is the all MS ID (local) address.
	AllMSIDLocal = 0xFFFFFD
	// AllMSIDZone is the all MS ID (zone) address.
	AllMSIDZone = 0xFFFFFE
	// AllMSID is the all MS ID (global) broadcast address.
	AllMSID = 0xFFFFFF
	// AddressNull is the null address.
	AddressNull = 0x000000
	// ChannelNull is the null channel number.
	ChannelNull = 0x001
	// DigitNull is the null BCD digit.
	DigitNull = 0b1111
)
