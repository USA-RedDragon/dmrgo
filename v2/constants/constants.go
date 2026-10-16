package constants

import "time"

// TAckWait is the recommended ACKWait timer (T_AckWait) for UU_Ans_Rsp. An
// MS that transmits a CSBK and waits for a response from the target
// retransmits the CSBK when this timer expires, if N_CSBKRetry allows. The
// value is chosen by the MS designer (ETSI TS 102 361-2 clause A.1).
const TAckWait = 360 * time.Millisecond

// TAckWaitSimulcast is the recommended minimum T_AckWait for simulcast
// systems (ETSI TS 102 361-2 clause A.1).
const TAckWaitSimulcast = 2 * time.Second

// TTO is the transmit timeout timer (T_TO) for DMR Tier I, a fixed value. It
// limits the time of a single transmission item (ETSI TS 102 361-2 clause
// A.1, clause 6.1).
const TTO = 180 * time.Second

// TTOMax is the maximum T_TO for DMR Tier II and Tier III, where the MS
// designer chooses a value from 0 to 180 s. A value of 0 disables the timer
// (ETSI TS 102 361-2 clause A.1, clause 6.1).
const TTOMax = 180 * time.Second

// TCallHang is the default call hang time (T_CallHt). After a voice call
// ends, a BS keeps the channel for this time, so that a new call to the
// same target continues the session (ETSI TS 102 361-1 clause F.1, clause
// 5.2.1.4).
const TCallHang = 3 * time.Second

// CTRHOTMax is the upper bound of the CT_CSBK transmission Random HoldOff
// Timer (CT_RHOT) when leader timing is unknown: a uniform distribution from
// 0 to 3.24 s. When leader timing is known, the range after a CT_CSBK
// transmission is 2.16 s to 3.24 s (ETSI TS 102 361-2 clause A.1).
const CTRHOTMax = 3240 * time.Millisecond

// CTRHOTIncrement is the increment of CT_RHOT (ETSI TS 102 361-2 clause
// A.1).
const CTRHOTIncrement = 60 * time.Millisecond

// NoLeader is the time an MS monitors the channel for wide area timing
// information after power up or a channel change, before it transmits a
// request (ETSI TS 102 361-2 clause A.1).
const NoLeader = 4*time.Minute + 30*time.Second

// SyncAge is the time wide area timing sync information is valid before the
// MS requires updated sync information (ETSI TS 102 361-2 clause A.1).
const SyncAge = 10 * time.Minute

// SyncAgeIncrement is the SyncAge increment (SAIncr) (ETSI TS 102 361-2
// clause A.1).
const SyncAgeIncrement = 500 * time.Millisecond

// SyncAgeWarning is the time an MS goes without updated sync information
// before it requests updated sync information (2*BeaconInterval) (ETSI TS
// 102 361-2 clause A.1).
const SyncAgeWarning = 9 * time.Minute

// TMSChanAuth is the recommended Channel Authorization timer (T_MS_ChanAuth)
// for single site deployments (ETSI TS 102 361-2 clause A.1).
const TMSChanAuth = 180 * time.Millisecond

// TMSChanAuthMultiSite is the recommended T_MS_ChanAuth for multi-site
// deployments (ETSI TS 102 361-2 clause A.1).
const TMSChanAuthMultiSite = 360 * time.Millisecond

// TBSChanAuthRspVoice is the recommended Channel Authorization Response
// timer (T_BS_ChanAuthRsp) for voice, single site and multi-site (ETSI TS
// 102 361-2 clause A.1).
const TBSChanAuthRspVoice = 720 * time.Millisecond

// TBSChanAuthRspCSBKSingle is the recommended T_BS_ChanAuthRsp for CSBK or
// data, single site (ETSI TS 102 361-2 clause A.1).
const TBSChanAuthRspCSBKSingle = 900 * time.Millisecond

// TBSChanAuthRspCSBKMulti is the recommended T_BS_ChanAuthRsp for CSBK or
// data, multi-site (ETSI TS 102 361-2 clause A.1).
const TBSChanAuthRspCSBKMulti = 1140 * time.Millisecond

// TBSChanAuthSel is the recommended Channel Authorization Selection timer
// (T_BS_ChanAuthSel), multi-site deployments only (ETSI TS 102 361-2 clause
// A.1).
const TBSChanAuthSel = 240 * time.Millisecond

// TRCTimer is the recommended T_RCtimer: the time an MS waits for the target
// MS of a Tier 2 RC Command to end its transmission before it retries the
// command. The value is chosen by the MS designer; simulcast systems can
// need a larger value (ETSI TS 102 361-2 clause A.1, clause 6.3).
const TRCTimer = 600 * time.Millisecond

// NCSBKRetry is the recommended CSBK retry limit (N_CSBKRetry) for UU_V_Req.
// The value is chosen by the MS designer and is application specific (ETSI
// TS 102 361-2 clause A.2).
const NCSBKRetry = 1

// BeaconDuration is the minimum duration of a CT_CSBK_Beacon and
// CT_CSBK_Prop transmission (ETSI TS 102 361-2 clause A.2).
const BeaconDuration = 600 * time.Millisecond

// BeaconInterval is the time between the start of two consecutive
// CT_CSBK_Beacon transmissions by the wide area timing leader MS (ETSI TS
// 102 361-2 clause A.2).
const BeaconInterval = 4*time.Minute + 30*time.Second

// CTDuration is the minimum duration of a CT_CSBK_Req and CT_CSBK_Resp
// transmission (ETSI TS 102 361-2 clause A.2).
const CTDuration = 180 * time.Millisecond

// TDataTxLimit is the recommended maximum Data Transmission Limit
// (T_DataTxLmt): the time an MS attempts to transmit an unconfirmed data
// message, or to transmit a confirmed data message and receive a reply. The
// value is chosen by the MS designer (ETSI TS 102 361-3 clause A.1).
const TDataTxLimit = 60 * time.Second

// TResponseWait is the recommended Confirmed Data Response Wait Limit
// (T_RspnsWait): the time an MS waits to receive the confirmed header packet
// data response. The value is chosen by the MS designer (ETSI TS 102 361-3
// clause A.1).
const TResponseWait = 180 * time.Millisecond

// TResponseWaitSimulcast is the recommended minimum T_RspnsWait for
// simulcast systems (ETSI TS 102 361-3 clause A.1).
const TResponseWaitSimulcast = 2 * time.Second

// THoldoffMax is the recommended maximum Random Holdoff Time (T_Holdoff) for
// unconfirmed and confirmed data. The MS randomly generates the timer
// duration over a range chosen by the MS designer, to minimize collisions
// when data messages are queued and the channel becomes idle (ETSI TS 102
// 361-3 clause A.1).
const THoldoffMax = 2 * time.Second

// TDataHangtime is the recommended Data Hangtime (T_DataHngtime), 3 traffic
// bursts: the time the BS transmits TD_LC PDUs to reserve the channel for a
// confirmed data response. The value is chosen by the BS designer (ETSI TS
// 102 361-3 clause A.1).
const TDataHangtime = 180 * time.Millisecond

// NRetryLimit is the recommended maximum Data Air Interface Retry Limit
// (N_RtryLmt): the number of times the DLL transmits and attempts to receive
// the confirmed data response from the target MS. The value is chosen by the
// MS designer (ETSI TS 102 361-3 clause A.2).
const NRetryLimit = 8

// TRandTCDefault is the default Trand_TC, the timeout for an MS attempting
// random access. The spec range is 2 s to 60 s, and this value is a library
// choice (ETSI TS 102 361-4 Table A.1).
const TRandTCDefault = 10 * time.Second

// TNoSignalDefault is the default T_Nosig, the timeout for entering hunting
// procedures if no TSCC is received. The spec range is 1 s to 15 s, and this
// value is a library choice (ETSI TS 102 361-4 Table A.1).
const TNoSignalDefault = 5 * time.Second

// TEmergTimerDefault is the default emergency call timer (T_EMERG_TIMER),
// token 24. The spec gives no default, so this value is a library choice
// (ETSI TS 102 361-4 Table A.2).
const TEmergTimerDefault = 120 * time.Second

// TEmergTimerMax is the longest finite T_EMERG_TIMER, token 510 (ETSI TS 102
// 361-4 Table A.2).
const TEmergTimerMax = 2315 * time.Minute

// TPacketTimerDefault is the default packet data call timer
// (T_PACKET_TIMER), token 6. The spec gives no default, so this value is a
// library choice (ETSI TS 102 361-4 Table A.3).
const TPacketTimerDefault = 10 * time.Second

// TPacketTimerMax is the longest finite T_PACKET_TIMER, token 30 (ETSI TS
// 102 361-4 Table A.3).
const TPacketTimerMax = 35 * time.Minute

// TMSMSTimerMax is the longest finite MS to MS call timer (T_MS-MS_TIMER),
// token 4094 (ETSI TS 102 361-4 Table A.4).
const TMSMSTimerMax = 3976 * time.Minute

// TMSLineTimerMax is the longest finite MS to line call timer
// (T_MS-LINE_TIMER), token 4094 (ETSI TS 102 361-4 Table A.5).
const TMSLineTimerMax = 3976 * time.Minute

// TPTimerDefault is the default TP_Timer, the timeout for a calling MS
// waiting for a call that requires a payload channel. The spec range is 4 s
// to 60 s, and this value is a library choice (ETSI TS 102 361-4 Table A.1).
const TPTimerDefault = 10 * time.Second

// TNPTimerDefault is the default TNP_Timer, the timeout for a calling MS
// waiting for a call that does not require a payload channel. The spec range
// is 2 s to 60 s, and this value is a library choice (ETSI TS 102 361-4
// Table A.1).
const TNPTimerDefault = 5 * time.Second

// TAwakeDefault is the default T_Awake, the time an MS stays awake after
// receiving a PDU. The spec range is 0.1 s to 60 s in steps of 0.1 s, and
// this value is a library choice (ETSI TS 102 361-4 Table A.1).
const TAwakeDefault = 1 * time.Second

// TVHangtimeDefault is the default TV_Hangtime, the payload voice hangtime
// timer. The spec range is 1 s to 60 s, and this value is a library choice
// (ETSI TS 102 361-4 Table A.1).
const TVHangtimeDefault = 4 * time.Second

// TVItemDefault is the default TV_Item, the payload voice maximum item
// timer. The spec range is 10 s to 60 s, and this value is a library choice
// (ETSI TS 102 361-4 Table A.1).
const TVItemDefault = 30 * time.Second

// TVInactiveDefault is the default TV_Inactive, the payload voice inactivity
// timer. The spec range is 0 s to 20 s, and this value is a library choice
// (ETSI TS 102 361-4 Table A.1).
const TVInactiveDefault = 5 * time.Second

// TDInactiveDefault is the default TD_Inactive, the payload data inactivity
// timer. The spec range is 0 s to 20 s, and this value is a library choice
// (ETSI TS 102 361-4 Table A.1).
const TDInactiveDefault = 5 * time.Second

// TDItemDefault is the default TD_Item, the payload packet data maximum item
// timer. The spec range is 1 s to 60 s, and this value is a library choice
// (ETSI TS 102 361-4 Table A.1).
const TDItemDefault = 30 * time.Second

// TDHangtimeDefault is the default TD_Hangtime, the payload data hangtime
// timer. The spec range is 1 s to 60 s, and this value is a library choice
// (ETSI TS 102 361-4 Table A.1).
const TDHangtimeDefault = 4 * time.Second

// TAnswerCallDefault is the default T_AnswerCall, the timeout for a called
// MS after it receives AHOY for FOACSU. The spec range is 2 s to 60 s, and
// this value is a library choice (ETSI TS 102 361-4 Table A.1).
const TAnswerCallDefault = 15 * time.Second

// TPendingDefault is the default T_Pending, the timeout for a called MS
// after it receives AHOY for OACSU. The spec range is 2 s to 60 s, and this
// value is a library choice (ETSI TS 102 361-4 Table A.1).
const TPendingDefault = 5 * time.Second

// TDeregDefault is the default T_dereg, the timer to de-register before
// abandon. The spec range is 0.2 s to 2 s in 0.1 s steps, and this value is
// a library choice (ETSI TS 102 361-4 Table A.1).
const TDeregDefault = 500 * time.Millisecond

// TBSInactiveDefault is the default T_BS_Inactive, the timer to hibernate if
// there is no inbound activity on an unregulated TSCC. The spec range is 1 s
// to 300 s, and this value is a library choice (ETSI TS 102 361-4 Table
// A.1).
const TBSInactiveDefault = 60 * time.Second

// TDenRegDefault is the default T_DENREG, the denied registration lifetime.
// It is coded in steps of 10 s: 0 is inactive, 1 to 1000 are 10 s to 10000
// s. The spec range is 0, or 10 s to 10000 s, and this value is a library
// choice (ETSI TS 102 361-4 Table A.1).
const TDenRegDefault = 60 * time.Second

// TALSDefault is the default T_ALS, the Ambient Listening Service timer for
// normal priority calls. The spec range is 10 s to 300 s, and this value is
// a library choice (ETSI TS 102 361-4 Table A.1).
const TALSDefault = 60 * time.Second

// TALSEDefault is the default T_ALS_E, the Ambient Listening Service timer
// for emergency priority calls. The spec range is 10 s to 14400 s, and this
// value is a library choice (ETSI TS 102 361-4 Table A.1).
const TALSEDefault = 300 * time.Second

// TALSRequestLifeSpanDefault is the default T_ALS_REQUEST_LIFE_SPAN, the
// maximum delay between C_AHOY and the subsequent C_GRANT before the called
// MS cancels the ALS request. The spec range is 1 s to 10 s, and this value
// is a library choice (ETSI TS 102 361-4 Table A.1).
const TALSRequestLifeSpanDefault = 5 * time.Second

// TALSRetransmitDelayDefault is the default T_ALS_RETRANSMIT_DELAY, the
// minimum delay before an ALS target MS re-keys after the ALS initiating
// party interrupts it. The spec range is 0 s to 5 s, and this value is a
// library choice (ETSI TS 102 361-4 Table A.1).
const TALSRetransmitDelayDefault = 1 * time.Second

// NDefaultNW is NDefault_NW, the NRand_Wait value an MS uses at switch on
// (ETSI TS 102 361-4 Table A.6).
const NDefaultNW = 5

// NRandNR is NRand_NR, the number of random access attempts for a normal and
// high priority service (ETSI TS 102 361-4 Table A.6).
const NRandNR = 6

// NRandNE is NRand_NE, the number of random access attempts for an emergency
// priority service (ETSI TS 102 361-4 Table A.6).
const NRandNE = 10

// NMaint is N_Maint, the number of P_MAINT PDUs an MS transmits to clear
// down the payload channel (ETSI TS 102 361-4 Table A.6).
const NMaint = 4

// NMaxCh is Nmax_Ch, the minimum number of channels in the Short Hunt List
// (ETSI TS 102 361-4 Table A.6).
const NMaxCh = 50

// ChPref is Ch_Pref, the channels that are marked as Preferred TSCC for
// Vote_Now (ETSI TS 102 361-4 Table A.6).
const ChPref = 50

// NSysErrDefault is the default NSYSerr, the number of C_SYScodes received
// that differ from the verified value. The spec range is 1 to 3, and this
// value is a library choice (ETSI TS 102 361-4 Table A.6).
const NSysErrDefault = 2

// DMRLADefault is the default DMRLA, the length of the SYS_AREA information
// field from the C_SYScode. The spec range is 1 to 10, and this value is a
// library choice (ETSI TS 102 361-4 Table A.6).
const DMRLADefault = 1

// VoteBlockDefault is the default VOTE_BLK, the number of TDMA frames for
// which the TSCC withdraws random access activity after a Vote Now Advice.
// The spec range is 2 to 10, and this value is a library choice (ETSI TS 102
// 361-4 Table A.6).
const VoteBlockDefault = 3

// The Tier III gateway addresses and identifiers. Address 0xFFFEC8 is
// reserved (ETSI TS 102 361-4 Table A.8).
const (
	// GatewayPSTNI (PSTNI) is the gateway address for services to the PSTN
	// using payload aligned timing.
	GatewayPSTNI = 0xFFFEC0
	// GatewayPABXI (PABXI) is the gateway address for services to the PABX
	// using payload aligned timing.
	GatewayPABXI = 0xFFFEC1
	// GatewayLINEI (LINEI) is the address for services to a Line Gateway
	// using payload aligned timing.
	GatewayLINEI = 0xFFFEC2
	// GatewayIPI (IPI) is the address for services to an IP Gateway using
	// payload aligned timing.
	GatewayIPI = 0xFFFEC3
	// GatewaySUPLI (SUPLI) is the address that identifies a supplementary
	// data service.
	GatewaySUPLI = 0xFFFEC4
	// GatewaySDMI (SDMI) is the address that identifies a UDT Short Data
	// service.
	GatewaySDMI = 0xFFFEC5
	// GatewayREGI (REGI) is the address that identifies a registration
	// service.
	GatewayREGI = 0xFFFEC6
	// GatewayMSI (MSI) is the gateway address for Call Diversion to an MS.
	GatewayMSI = 0xFFFEC7
	// GatewayDIVERTI (DIVERTI) is the address that identifies a call
	// diversion cancellation.
	GatewayDIVERTI = 0xFFFEC9
	// GatewayTSI (TSI) is the address of the TS.
	GatewayTSI = 0xFFFECA
	// GatewayDISPATI (DISPATI) is the address of the system dispatcher using
	// payload aligned timing.
	GatewayDISPATI = 0xFFFECB
	// GatewaySTUNI (STUNI) is the MS Stun/Revive identifier.
	GatewaySTUNI = 0xFFFECC
	// GatewayAUTHI (AUTHI) is the Authentication identifier.
	GatewayAUTHI = 0xFFFECD
	// GatewayGPI (GPI) is the gateway address for Call Diversion to a
	// Talkgroup.
	GatewayGPI = 0xFFFECE
	// GatewayKILLI (KILLI) is the MS KILL identifier.
	GatewayKILLI = 0xFFFECF
	// GatewayPSTNDI (PSTNDI) is the gateway address for services to the PSTN
	// using payload offset timing.
	GatewayPSTNDI = 0xFFFED0
	// GatewayPABXDI (PABXDI) is the gateway address for services to the PABX
	// using payload offset timing.
	GatewayPABXDI = 0xFFFED1
	// GatewayLINEDI (LINEDI) is the address for services to a Line Gateway
	// using payload offset timing.
	GatewayLINEDI = 0xFFFED2
	// GatewayDISPATDI (DISPATDI) is the address of the system dispatcher
	// using payload offset timing.
	GatewayDISPATDI = 0xFFFED3
	// GatewayALLMSI (ALLMSI) is the totality of all individual MS and
	// talkgroups.
	GatewayALLMSI = 0xFFFED4
	// GatewayIPDI (IPDI) is the address for services to an IP gateway using
	// payload offset timing.
	GatewayIPDI = 0xFFFED5
	// GatewayDGNAI (DGNAI) is the address that identifies the Dynamic Group
	// Number Assignment.
	GatewayDGNAI = 0xFFFED6
	// GatewayTATTSI (TATTSI) is the address that identifies the Talkgroup
	// Subscription/Attachment Service.
	GatewayTATTSI = 0xFFFED7
	// AllMSIDLocal (ALLMSIDL) addresses all MS at one site as a talkgroup.
	AllMSIDLocal = 0xFFFFFD
	// AllMSIDZone (ALLMSIDZ) addresses all MS in a subset of the system's
	// sites as a talkgroup.
	AllMSIDZone = 0xFFFFFE
	// AllMSID (ALLMSID) addresses all MS (AllCall) in every site in the
	// system as a talkgroup.
	AllMSID = 0xFFFFFF
	// AddressNull (ADRNULL) is an ID that is not assigned to any entity.
	AddressNull = 0x000000
	// ChannelNull (CHNULL) is a logical physical channel that is not
	// assigned. Logical channels in use are 1 to 4095 (Table A.6
	// Low_Comp_Ch, High_Comp_Ch).
	ChannelNull = 0x000
	// DigitNull (DigitNULL) fills unused BCD information elements.
	DigitNull = 0b1111
)
