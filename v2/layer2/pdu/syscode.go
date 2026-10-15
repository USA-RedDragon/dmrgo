package pdu

import (
	"fmt"
)

// NetworkModel is the MODEL field of a System Identity Code
// (ETSI TS 102 361-4 Table 6.5).
type NetworkModel uint8

const (
	NetworkModelTiny  NetworkModel = 0b00
	NetworkModelSmall NetworkModel = 0b01
	NetworkModelLarge NetworkModel = 0b10
	NetworkModelHuge  NetworkModel = 0b11
)

// NetBits returns the width of the NET field for the model. The SITE field
// takes the other 12 - NetBits bits.
func (m NetworkModel) NetBits() int {
	return netSiteBits(uint8(m))
}

// SiteBits returns the width of the SITE field for the model.
func (m NetworkModel) SiteBits() int {
	return netSiteLen - m.NetBits()
}

// SysCode is the 16-bit C_SYScode (ETSI TS 102 361-4 §6.3.2.2.1.1,
// Figure 6.15): MODEL (2 bits), NET and SITE (12 bits split by MODEL),
// then PAR (2 bits). C_SYS_Parms and P_SYS_Parms in the CACH carry only the
// 14 most significant bits, without PAR (§5.1.1).
type SysCode struct {
	Model NetworkModel
	Net   uint16
	Site  uint16
	Par   uint8
}

func (s SysCode) check() error {
	if s.Model > NetworkModelHuge {
		return fmt.Errorf("network model %d does not fit in 2 bits", s.Model)
	}
	if s.Net>>s.Model.NetBits() != 0 {
		return fmt.Errorf("NET %d does not fit in %d bits", s.Net, s.Model.NetBits())
	}
	if s.Site>>s.Model.SiteBits() != 0 {
		return fmt.Errorf("SITE %d does not fit in %d bits", s.Site, s.Model.SiteBits())
	}
	if s.Par > 3 {
		return fmt.Errorf("PAR %d does not fit in 2 bits", s.Par)
	}
	return nil
}

// Pack returns the 16-bit C_SYScode, as in the SysIdentCode field of
// C_ALOHA and C_BCAST. It returns an error when a field does not fit.
func (s SysCode) Pack() (uint16, error) {
	if err := s.check(); err != nil {
		return 0, err
	}
	return uint16(s.Model)<<14 | s.Net<<(2+s.Model.SiteBits()) | s.Site<<2 | uint16(s.Par), nil
}

// Pack14 returns the 14 most significant bits of the C_SYScode, the form
// in the CACH Short LC. Par is ignored.
func (s SysCode) Pack14() (uint16, error) {
	s.Par = 0
	v, err := s.Pack()
	return v >> 2, err
}

// NetSite returns the 12 NET and SITE bits as in the NetSiteRaw field of
// ShortLCCSysParms and ShortLCPSysParms.
func (s SysCode) NetSite() (uint16, error) {
	v, err := s.Pack14()
	return v & 0xFFF, err
}

// UnpackSysCode splits a 16-bit C_SYScode into its fields.
func UnpackSysCode(v uint16) SysCode {
	model := NetworkModel(v >> 14)
	siteBits := model.SiteBits()
	return SysCode{
		Model: model,
		Net:   (v >> (2 + siteBits)) & (1<<model.NetBits() - 1),
		Site:  (v >> 2) & (1<<siteBits - 1),
		Par:   uint8(v & 3),
	}
}

// UnpackSysCode14 splits the 14-bit CACH form of a C_SYScode. Par is 0.
func UnpackSysCode14(v uint16) SysCode {
	return UnpackSysCode((v & 0x3FFF) << 2)
}

func sysCodeFromShortLC(model uint8, netSite uint16) SysCode {
	return UnpackSysCode14(uint16(model&3)<<12 | netSite&0xFFF)
}

// SysCode returns the MODEL, NET and SITE carried in the C_SYS_Parms.
func (s *ShortLCCSysParms) SysCode() SysCode {
	return sysCodeFromShortLC(s.MODEL, s.NetSiteRaw)
}

// SetSysCode sets MODEL and NetSiteRaw from c. c.Par is ignored.
func (s *ShortLCCSysParms) SetSysCode(c SysCode) error {
	netSite, err := c.NetSite()
	if err != nil {
		return err
	}
	s.MODEL, s.NetSiteRaw = uint8(c.Model), netSite
	return nil
}

// SysCode returns the MODEL, NET and SITE carried in the P_SYS_Parms.
func (s *ShortLCPSysParms) SysCode() SysCode {
	return sysCodeFromShortLC(s.MODEL, s.NetSiteRaw)
}

// SetSysCode sets MODEL and NetSiteRaw from c. c.Par is ignored.
func (s *ShortLCPSysParms) SetSysCode(c SysCode) error {
	netSite, err := c.NetSite()
	if err != nil {
		return err
	}
	s.MODEL, s.NetSiteRaw = uint8(c.Model), netSite
	return nil
}
