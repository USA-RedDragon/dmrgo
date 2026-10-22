package mmdvm

import "github.com/USA-RedDragon/dmrgo/v2/enums"

// StatusV2 is the payload of a CommandGetStatus reply of protocol 2 (MMDVM
// firmware): the modem state, its flags and the free space of each TX
// buffer.
//
//dmr:frame 17
//dmr:payload
//dmr:const 8-8=0
//dmr:const 16-23=0
//dmr:const 72-79=0
//dmr:const 96-127=0
//dmr:const 128-135=0
type StatusV2 struct {
	// State is the mode or calibration state of the modem.
	State Mode `dmr:"bits:0-7"`
	// CD reports a carrier on the receiver.
	CD bool `dmr:"bit:9"`
	// DACOverflow reports that the DAC levels overflowed.
	DACOverflow bool `dmr:"bit:10"`
	// Lockout reports the lockout input.
	Lockout bool `dmr:"bit:11"`
	// TXOverflow reports that the TX buffer overflowed.
	TXOverflow bool `dmr:"bit:12"`
	// RXOverflow reports that the RX buffer overflowed.
	RXOverflow bool `dmr:"bit:13"`
	// ADCOverflow reports that the ADC levels overflowed.
	ADCOverflow bool `dmr:"bit:14"`
	// TX reports that the modem transmits.
	TX bool `dmr:"bit:15"`
	// DStarSpace is the free space of the D-Star TX buffer.
	DStarSpace uint8 `dmr:"bits:24-31"`
	// DMRSpace1 is the free space of the slot 1 TX buffer, or 10 in simplex.
	DMRSpace1 uint8 `dmr:"bits:32-39"`
	// DMRSpace2 is the free space of the slot 2 TX buffer, or of the DMO
	// buffer in simplex.
	DMRSpace2 uint8 `dmr:"bits:40-47"`
	// YSFSpace is the free space of the YSF TX buffer.
	YSFSpace uint8 `dmr:"bits:48-55"`
	// P25Space is the free space of the P25 TX buffer.
	P25Space uint8 `dmr:"bits:56-63"`
	// NXDNSpace is the free space of the NXDN TX buffer.
	NXDNSpace uint8 `dmr:"bits:64-71"`
	// FMSpace is the free space of the FM TX buffer.
	FMSpace uint8 `dmr:"bits:80-87"`
	// POCSAGSpace is the free space of the POCSAG TX buffer.
	POCSAGSpace uint8 `dmr:"bits:88-95"`
}

// StatusV1 is the payload of a CommandGetStatus reply of protocol 1
// (MMDVM_HS firmware): the enabled modes, the modem state, its flags and the
// free space of each TX buffer.
//
//dmr:frame 11
//dmr:payload
//dmr:const 1-1=0
//dmr:const 16-16=0
type StatusV1 struct {
	// M17 reports that M17 is enabled.
	M17 bool `dmr:"bit:0"`
	// POCSAG reports that POCSAG is enabled.
	POCSAG bool `dmr:"bit:2"`
	// NXDN reports that NXDN is enabled.
	NXDN bool `dmr:"bit:3"`
	// P25 reports that P25 is enabled.
	P25 bool `dmr:"bit:4"`
	// YSF reports that YSF is enabled.
	YSF bool `dmr:"bit:5"`
	// DMR reports that DMR is enabled.
	DMR bool `dmr:"bit:6"`
	// DStar reports that D-Star is enabled.
	DStar bool `dmr:"bit:7"`
	// State is the mode or calibration state of the modem.
	State Mode `dmr:"bits:8-15"`
	// CD reports a carrier on the receiver; MMDVM_HS does not set it.
	CD bool `dmr:"bit:17"`
	// DACOverflow reports that the DAC levels overflowed; MMDVM_HS does not
	// set it.
	DACOverflow bool `dmr:"bit:18"`
	// Lockout reports the lockout input; MMDVM_HS does not set it.
	Lockout bool `dmr:"bit:19"`
	// TXOverflow reports that the TX buffer overflowed.
	TXOverflow bool `dmr:"bit:20"`
	// RXOverflow reports that the RX buffer overflowed.
	RXOverflow bool `dmr:"bit:21"`
	// ADCOverflow reports that the ADC levels overflowed; MMDVM_HS does not
	// set it.
	ADCOverflow bool `dmr:"bit:22"`
	// TX reports that the modem transmits.
	TX bool `dmr:"bit:23"`
	// DStarSpace is the free space of the D-Star TX buffer.
	DStarSpace uint8 `dmr:"bits:24-31"`
	// DMRSpace1 is the free space of the slot 1 TX buffer, or 10 in simplex.
	DMRSpace1 uint8 `dmr:"bits:32-39"`
	// DMRSpace2 is the free space of the slot 2 TX buffer, or of the DMO
	// buffer in simplex.
	DMRSpace2 uint8 `dmr:"bits:40-47"`
	// YSFSpace is the free space of the YSF TX buffer.
	YSFSpace uint8 `dmr:"bits:48-55"`
	// P25Space is the free space of the P25 TX buffer.
	P25Space uint8 `dmr:"bits:56-63"`
	// NXDNSpace is the free space of the NXDN TX buffer.
	NXDNSpace uint8 `dmr:"bits:64-71"`
	// POCSAGSpace is the free space of the POCSAG TX buffer.
	POCSAGSpace uint8 `dmr:"bits:72-79"`
	// M17Space is the free space of the M17 TX buffer.
	M17Space uint8 `dmr:"bits:80-87"`
}

// ConfigV2 is the payload of a CommandSetConfig frame of protocol 2, as
// MMDVMHost setConfig2 writes it and the MMDVM firmware reads it. Levels
// are 0 to 255 for 0 to 100 percent.
//
//dmr:frame 37
//dmr:payload
//dmr:const 1-1=0
//dmr:const 8-9=0
//dmr:const 16-22=0
//dmr:const 112-119=0
//dmr:const 136-159=0
//dmr:const 184-199=0
//dmr:const 201-207=0
//dmr:const 232-263=0
//dmr:const 264-295=0
type ConfigV2 struct {
	// Simplex selects simplex (DMO) operation instead of duplex.
	Simplex bool `dmr:"bit:0"`
	// UseCOSAsLockout uses the COS input as lockout.
	UseCOSAsLockout bool `dmr:"bit:2"`
	// Debug makes the modem send debug frames.
	Debug bool `dmr:"bit:3"`
	// YSFLoDev selects the low YSF deviation.
	YSFLoDev bool `dmr:"bit:4"`
	// PTTInvert inverts the PTT output.
	PTTInvert bool `dmr:"bit:5"`
	// TXInvert inverts the TX audio.
	TXInvert bool `dmr:"bit:6"`
	// RXInvert inverts the RX audio.
	RXInvert bool `dmr:"bit:7"`
	// FM enables FM.
	FM bool `dmr:"bit:10"`
	// NXDN enables NXDN.
	NXDN bool `dmr:"bit:11"`
	// P25 enables P25.
	P25 bool `dmr:"bit:12"`
	// YSF enables YSF.
	YSF bool `dmr:"bit:13"`
	// DMR enables DMR.
	DMR bool `dmr:"bit:14"`
	// DStar enables D-Star.
	DStar bool `dmr:"bit:15"`
	// POCSAG enables POCSAG.
	POCSAG bool `dmr:"bit:23"`
	// TXDelay is the TX delay in units of 10 ms, at most 50.
	TXDelay uint8 `dmr:"bits:24-31"`
	// State is the state to start in, ModeIdle from MMDVMHost.
	State Mode `dmr:"bits:32-39"`
	// TXDCOffset is the TX DC offset plus 128.
	TXDCOffset uint8 `dmr:"bits:40-47"`
	// RXDCOffset is the RX DC offset plus 128.
	RXDCOffset uint8 `dmr:"bits:48-55"`
	// RXLevel is the RX level.
	RXLevel uint8 `dmr:"bits:56-63"`
	// CWIDTXLevel is the CW ID TX level.
	CWIDTXLevel uint8 `dmr:"bits:64-71"`
	// DStarTXLevel is the D-Star TX level.
	DStarTXLevel uint8 `dmr:"bits:72-79"`
	// DMRTXLevel is the DMR TX level.
	DMRTXLevel uint8 `dmr:"bits:80-87"`
	// YSFTXLevel is the YSF TX level.
	YSFTXLevel uint8 `dmr:"bits:88-95"`
	// P25TXLevel is the P25 TX level.
	P25TXLevel uint8 `dmr:"bits:96-103"`
	// NXDNTXLevel is the NXDN TX level.
	NXDNTXLevel uint8 `dmr:"bits:104-111"`
	// POCSAGTXLevel is the POCSAG TX level.
	POCSAGTXLevel uint8 `dmr:"bits:120-127"`
	// FMTXLevel is the FM TX level.
	FMTXLevel uint8 `dmr:"bits:128-135"`
	// YSFTXHang is the YSF TX hang time in seconds.
	YSFTXHang uint8 `dmr:"bits:160-167"`
	// P25TXHang is the P25 TX hang time in seconds.
	P25TXHang uint8 `dmr:"bits:168-175"`
	// NXDNTXHang is the NXDN TX hang time in seconds.
	NXDNTXHang uint8 `dmr:"bits:176-183"`
	// DMRTrunking makes the modem a DMR Tier III control channel.
	DMRTrunking bool `dmr:"bit:200"`
	// DMRColorCode is the DMR color code, 0 to 15.
	DMRColorCode enums.ColorCode `dmr:"bits:208-215"`
	// DMRDelay is the DMR delay in symbols.
	DMRDelay uint8 `dmr:"bits:216-223"`
	// OscOffset is 128; the field was the oscillator offset.
	OscOffset uint8 `dmr:"bits:224-231"`
}

// ConfigV1 is the payload of a CommandSetConfig frame of protocol 1, as
// MMDVMHost setConfig1 writes it and the MMDVM_HS firmware reads it.
// Levels are 0 to 255 for 0 to 100 percent.
//
//dmr:frame 23
//dmr:payload
//dmr:const 1-1=0
//dmr:const 8-8=0
type ConfigV1 struct {
	// Simplex selects simplex (DMO) operation instead of duplex.
	Simplex bool `dmr:"bit:0"`
	// UseCOSAsLockout uses the COS input as lockout.
	UseCOSAsLockout bool `dmr:"bit:2"`
	// Debug makes the modem send debug frames.
	Debug bool `dmr:"bit:3"`
	// YSFLoDev selects the low YSF deviation.
	YSFLoDev bool `dmr:"bit:4"`
	// PTTInvert inverts the PTT output.
	PTTInvert bool `dmr:"bit:5"`
	// TXInvert inverts the TX audio.
	TXInvert bool `dmr:"bit:6"`
	// RXInvert inverts the RX audio.
	RXInvert bool `dmr:"bit:7"`
	// M17 enables M17 (MMDVM_HS).
	M17 bool `dmr:"bit:9"`
	// POCSAG enables POCSAG.
	POCSAG bool `dmr:"bit:10"`
	// NXDN enables NXDN.
	NXDN bool `dmr:"bit:11"`
	// P25 enables P25.
	P25 bool `dmr:"bit:12"`
	// YSF enables YSF.
	YSF bool `dmr:"bit:13"`
	// DMR enables DMR.
	DMR bool `dmr:"bit:14"`
	// DStar enables D-Star.
	DStar bool `dmr:"bit:15"`
	// TXDelay is the TX delay in units of 10 ms.
	TXDelay uint8 `dmr:"bits:16-23"`
	// State is the state to start in, ModeIdle from MMDVMHost.
	State Mode `dmr:"bits:24-31"`
	// RXLevel is the RX level.
	RXLevel uint8 `dmr:"bits:32-39"`
	// CWIDTXLevel is the CW ID TX level.
	CWIDTXLevel uint8 `dmr:"bits:40-47"`
	// DMRColorCode is the DMR color code, 0 to 15.
	DMRColorCode enums.ColorCode `dmr:"bits:48-55"`
	// DMRDelay is the DMR delay in symbols.
	DMRDelay uint8 `dmr:"bits:56-63"`
	// OscOffset is 128; the field was the oscillator offset.
	OscOffset uint8 `dmr:"bits:64-71"`
	// DStarTXLevel is the D-Star TX level.
	DStarTXLevel uint8 `dmr:"bits:72-79"`
	// DMRTXLevel is the DMR TX level.
	DMRTXLevel uint8 `dmr:"bits:80-87"`
	// YSFTXLevel is the YSF TX level.
	YSFTXLevel uint8 `dmr:"bits:88-95"`
	// P25TXLevel is the P25 TX level.
	P25TXLevel uint8 `dmr:"bits:96-103"`
	// TXDCOffset is the TX DC offset plus 128.
	TXDCOffset uint8 `dmr:"bits:104-111"`
	// RXDCOffset is the RX DC offset plus 128.
	RXDCOffset uint8 `dmr:"bits:112-119"`
	// NXDNTXLevel is the NXDN TX level.
	NXDNTXLevel uint8 `dmr:"bits:120-127"`
	// YSFTXHang is the YSF TX hang time in seconds.
	YSFTXHang uint8 `dmr:"bits:128-135"`
	// POCSAGTXLevel is the POCSAG TX level.
	POCSAGTXLevel uint8 `dmr:"bits:136-143"`
	// FMTXLevel is the FM TX level; MMDVM_HS does not read it.
	FMTXLevel uint8 `dmr:"bits:144-151"`
	// P25TXHang is the P25 TX hang time in seconds.
	P25TXHang uint8 `dmr:"bits:152-159"`
	// NXDNTXHang is the NXDN TX hang time in seconds.
	NXDNTXHang uint8 `dmr:"bits:160-167"`
	// M17TXLevel is the M17 TX level (MMDVM_HS), 0 from MMDVMHost
	// 9384d68.
	M17TXLevel uint8 `dmr:"bits:168-175"`
	// M17TXHang is the M17 TX hang time in seconds from the MMDVMHost of
	// Pi-Star, 0 from MMDVMHost 9384d68; MMDVM_HS does not read it.
	M17TXHang uint8 `dmr:"bits:176-183"`
}

// SetFreq is the payload of a CommandSetFreq frame: the RX and TX
// frequencies, and from current hosts the RF level and the POCSAG TX
// frequency.
//
//dmr:frame 9 10 14
//dmr:payload
type SetFreq struct {
	// Reserved is octet 0, 0x00 from MMDVMHost.
	Reserved uint8 `dmr:"bits:0-7"`
	// RX is the receive frequency in Hz.
	RX uint32 `dmr:"bits:8-39,le"`
	// TX is the transmit frequency in Hz.
	TX uint32 `dmr:"bits:40-71,le"`
	// RFLevel is the RF power, 0 to 255 (10 and 14 octets).
	RFLevel uint8 `dmr:"bits:72-79"`
	// POCSAG is the POCSAG TX frequency in Hz (14 octets).
	POCSAG uint32 `dmr:"bits:80-111,le"`
	// Size is the length of the payload: 9, 10 or 14, or 0 for 9.
	Size uint8 `dmr:"-"`
}

// SetMode is the payload of a CommandSetMode frame.
//
//dmr:frame 1
//dmr:payload
type SetMode struct {
	// Mode is the mode to change to.
	Mode Mode `dmr:"bits:0-7"`
}

// Ack is the payload of a CommandAck frame from the modem.
//
//dmr:frame 1
//dmr:payload
type Ack struct {
	// Command is the command that the modem accepted.
	Command Command `dmr:"bits:0-7"`
}

// Nak is the payload of a CommandNak frame from the modem.
//
//dmr:frame 2
//dmr:payload
type Nak struct {
	// Command is the command that the modem refused.
	Command Command `dmr:"bits:0-7"`
	// Reason is the error code of the firmware, for example 4 for a
	// payload that it rejects.
	Reason uint8 `dmr:"bits:8-15"`
}

// DMRStart is the payload of a CommandDMRStart frame: it starts or stops
// the duplex transmitter.
//
//dmr:frame 1
//dmr:payload
//dmr:const 0-6=0
type DMRStart struct {
	// TX starts the transmitter; false stops it.
	TX bool `dmr:"bit:7"`
}

// DMRAbort is the payload of a CommandDMRAbort frame: it drops the queued
// bursts of one slot.
//
//dmr:frame 1
//dmr:payload
type DMRAbort struct {
	// Slot is the slot, Slot1 (1) or Slot2 (2).
	Slot enums.Slot `dmr:"bits:0-7,enum"`
}

// DMRShortLC is the payload of a CommandDMRShortLC frame: the Short LC that
// the modem sends in the CACH, as MMDVMHost CDMRShortLC encodes it.
// NewDMRShortLC builds it from a pdu.ShortLC, and ShortLC decodes it.
//
//dmr:frame 9
//dmr:payload
type DMRShortLC struct {
	// LC is the 68 coded and interleaved bits of the Short LC, first bit
	// first, then 4 unused bits.
	LC [9]byte `dmr:"bits:0-71,packed"`
}
