package pdu

import "github.com/USA-RedDragon/dmrgo/v2/bit"

// CBroadcastPDU is the old name of BroadcastPDU.
//
// Deprecated: use BroadcastPDU.
type CBroadcastPDU = BroadcastPDU

// CheckConfirmedCRC9 returns s.ConfirmedCRC9OK().
//
// Deprecated: use ConfirmedCRC9OK.
func (s *Rate12Data) CheckConfirmedCRC9() bool { return s.ConfirmedCRC9OK() }

// CheckConfirmedCRC9 returns s.ConfirmedCRC9OK().
//
// Deprecated: use ConfirmedCRC9OK.
func (s *Rate34Data) CheckConfirmedCRC9() bool { return s.ConfirmedCRC9OK() }

// CheckConfirmedCRC9 returns s.ConfirmedCRC9OK().
//
// Deprecated: use ConfirmedCRC9OK.
func (s *Rate1Data) CheckConfirmedCRC9() bool { return s.ConfirmedCRC9OK() }

// CheckMBCLastBlockCRC returns MBCLastBlockCRCOK(blocks...).
//
// Deprecated: use MBCLastBlockCRCOK.
func CheckMBCLastBlockCRC(blocks ...[96]bit.Bit) bool { return MBCLastBlockCRCOK(blocks...) }

// FullLinkControlFromBytes returns DecodeFullLinkControlBytes(lc).
//
// Deprecated: use DecodeFullLinkControlBytes.
func FullLinkControlFromBytes(lc [9]byte) (FullLinkControl, error) {
	return DecodeFullLinkControlBytes(lc)
}

// UnpackSysCode returns the SysCode that SysCode.Unpack sets from v.
//
// Deprecated: use SysCode.Unpack.
func UnpackSysCode(v uint16) SysCode {
	var s SysCode
	s.Unpack(v)
	return s
}

// UnpackSysCode14 returns the SysCode that SysCode.Unpack14 sets from v.
//
// Deprecated: use SysCode.Unpack14.
func UnpackSysCode14(v uint16) SysCode {
	var s SysCode
	s.Unpack14(v)
	return s
}
