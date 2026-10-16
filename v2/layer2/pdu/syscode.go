package pdu

import "github.com/USA-RedDragon/dmrgo/v2"

// NetworkModel is the MODEL field of a System Identity Code
// (ETSI TS 102 361-4 Table 6.5).
type NetworkModel uint8

// NetworkModel values. The NET field width is in parentheses.
const (
	NetworkModelTiny  NetworkModel = 0b00 // Tiny (9 bits)
	NetworkModelSmall NetworkModel = 0b01 // Small (7 bits)
	NetworkModelLarge NetworkModel = 0b10 // Large (4 bits)
	NetworkModelHuge  NetworkModel = 0b11 // Huge (2 bits)
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

// SysCode is the 16-bit C_SYScode: MODEL (2 bits), NET and SITE (12 bits
// split by MODEL), then PAR (2 bits) (ETSI TS 102 361-4 clause 6.3.2.2.1.1,
// Figure 6.15). C_SYS_Parms and P_SYS_Parms in the CACH carry only the 14
// most significant bits, without PAR (ETSI TS 102 361-4 clause 5.1.1).
type SysCode struct {
	Model NetworkModel // network model
	Net   uint16       // NET, NetBits wide
	Site  uint16       // SITE, SiteBits wide
	Par   uint8        // 2-bit PAR
}

func (s SysCode) check() error {
	if s.Model > NetworkModelHuge {
		return errSysCodeModelRange
	}
	if s.Net>>s.Model.NetBits() != 0 {
		return sysCodeNetRange(s.Model)
	}
	if s.Site>>s.Model.SiteBits() != 0 {
		return sysCodeSiteRange(s.Model)
	}
	if s.Par > 3 {
		return errSysCodeParRange
	}
	return nil
}

// Pack returns the 16-bit C_SYScode, as in the SysIdentCode field of
// C_ALOHA and C_BCAST. It returns a *dmrgo.FieldRangeError when a field does
// not fit its bits.
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

// Unpack sets s to the fields of the 16-bit C_SYScode v. It is the inverse
// of Pack.
func (s *SysCode) Unpack(v uint16) {
	model := NetworkModel(v >> 14)
	siteBits := model.SiteBits()
	*s = SysCode{
		Model: model,
		Net:   (v >> (2 + siteBits)) & (1<<model.NetBits() - 1),
		Site:  (v >> 2) & (1<<siteBits - 1),
		Par:   uint8(v & 3),
	}
}

// Unpack14 sets s to the fields of the 14-bit CACH form v of a C_SYScode,
// with Par 0. It is the inverse of Pack14.
func (s *SysCode) Unpack14(v uint16) {
	s.Unpack((v & 0x3FFF) << 2)
}

func sysCodeFromShortLC(model uint8, netSite uint16) SysCode {
	var s SysCode
	s.Unpack14(uint16(model&3)<<12 | netSite&0xFFF)
	return s
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

// sysCodeNetRange returns the error of a NET that does not fit in the NET
// bits of model m.
func sysCodeNetRange(m NetworkModel) error {
	switch m {
	case NetworkModelTiny:
		return errSysCodeNetRangeTiny
	case NetworkModelSmall:
		return errSysCodeNetRangeSmall
	case NetworkModelLarge:
		return errSysCodeNetRangeLarge
	default:
		return errSysCodeNetRangeHuge
	}
}

// sysCodeSiteRange returns the error of a SITE that does not fit in the
// SITE bits of model m.
func sysCodeSiteRange(m NetworkModel) error {
	switch m {
	case NetworkModelTiny:
		return errSysCodeSiteRangeTiny
	case NetworkModelSmall:
		return errSysCodeSiteRangeSmall
	case NetworkModelLarge:
		return errSysCodeSiteRangeLarge
	default:
		return errSysCodeSiteRangeHuge
	}
}

// The Type of the SYSCODE errors, and the Field of NET and SITE.
const (
	sysCodeType = "SysCode"
	sysCodeNet  = "Net"
	sysCodeSite = "Site"
)

// The errors of a SYSCODE field whose value does not fit in its bits. NET
// and SITE have one for each network model, which sets their widths.
var (
	errSysCodeModelRange     = &dmrgo.FieldRangeError{Type: sysCodeType, Field: "Model", Bits: 2}
	errSysCodeParRange       = &dmrgo.FieldRangeError{Type: sysCodeType, Field: "Par", Bits: 2}
	errSysCodeNetRangeTiny   = &dmrgo.FieldRangeError{Type: sysCodeType, Field: sysCodeNet, Bits: 9}
	errSysCodeNetRangeSmall  = &dmrgo.FieldRangeError{Type: sysCodeType, Field: sysCodeNet, Bits: 7}
	errSysCodeNetRangeLarge  = &dmrgo.FieldRangeError{Type: sysCodeType, Field: sysCodeNet, Bits: 4}
	errSysCodeNetRangeHuge   = &dmrgo.FieldRangeError{Type: sysCodeType, Field: sysCodeNet, Bits: 2}
	errSysCodeSiteRangeTiny  = &dmrgo.FieldRangeError{Type: sysCodeType, Field: sysCodeSite, Bits: 3}
	errSysCodeSiteRangeSmall = &dmrgo.FieldRangeError{Type: sysCodeType, Field: sysCodeSite, Bits: 5}
	errSysCodeSiteRangeLarge = &dmrgo.FieldRangeError{Type: sysCodeType, Field: sysCodeSite, Bits: 8}
	errSysCodeSiteRangeHuge  = &dmrgo.FieldRangeError{Type: sysCodeType, Field: sysCodeSite, Bits: 10}
)
