package dmrd

// MMDVMConfig is the 302-octet RPTC configuration message of DialectMMDVM,
// as a view: its methods read and write the fields in place. DMRGateway
// writes the numbers zero padded, which the getters read as decimal.
type MMDVMConfig [302]byte

// HBRPConfig is the 306-octet RPTC configuration message of DialectHBRP, as
// a view. The repeater ID is 8 hex digits after the callsign, the
// description has 20 characters and there is no slots digit.
type HBRPConfig [306]byte

// MMDVMConfigOf returns p as an MMDVMConfig without a copy, so the
// MMDVMConfig is valid as long as p is. It returns ErrShort, ErrLength, or
// ErrSignature when p does not start with "RPTC".
func MMDVMConfigOf(p []byte) (*MMDVMConfig, error) {
	if len(p) < len(MMDVMConfig{}) {
		return nil, ErrShort
	}
	if len(p) != len(MMDVMConfig{}) {
		return nil, ErrLength
	}
	c := (*MMDVMConfig)(p)
	if !c.magicOK() {
		return nil, ErrSignature
	}
	return c, nil
}

// HBRPConfigOf returns p as an HBRPConfig without a copy, with the errors
// of MMDVMConfigOf.
func HBRPConfigOf(p []byte) (*HBRPConfig, error) {
	if len(p) < len(HBRPConfig{}) {
		return nil, ErrShort
	}
	if len(p) != len(HBRPConfig{}) {
		return nil, ErrLength
	}
	c := (*HBRPConfig)(p)
	if !c.magicOK() {
		return nil, ErrSignature
	}
	return c, nil
}
