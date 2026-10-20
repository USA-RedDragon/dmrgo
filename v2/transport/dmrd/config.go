package dmrd

// Config is the 302-octet RPTC configuration message of DialectMMDVM, as a
// view: its methods read and write the fields in place. DMRGateway writes
// the numbers zero padded, which the getters read as decimal.
type Config [302]byte

// HBRPConfig is the 306-octet RPTC configuration message of DialectHBRP, as
// a view. The repeater ID is 8 hex digits after the callsign, the
// description has 20 characters and there is no slots digit.
type HBRPConfig [306]byte

// ConfigOf returns p as a Config without a copy, so the Config is valid as
// long as p is. It returns ErrShort, ErrLength, or ErrSignature when p does
// not start with "RPTC".
func ConfigOf(p []byte) (*Config, error) {
	if len(p) < len(Config{}) {
		return nil, ErrShort
	}
	if len(p) != len(Config{}) {
		return nil, ErrLength
	}
	c := (*Config)(p)
	if !c.magicOK() {
		return nil, ErrSignature
	}
	return c, nil
}

// HBRPConfigOf returns p as an HBRPConfig without a copy, with the errors
// of ConfigOf.
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
