package dmrd

import "github.com/USA-RedDragon/dmrgo/v2"

// Control is a login, keepalive, close, beacon, report, options or local
// link message: every message of a dialect but DMRD, DMRA, DMRG, RPTC and
// the OpenBridge messages.
type Control struct {
	// Command is the message.
	Command Command
	// ID is the repeater ID. In a DialectMMDVM RPTACK that answers RPTL it
	// holds the salt instead, as DMRGateway reads it.
	ID uint32
	// Salt is the salt of a DialectHBRP MSTACK that answers RPTL. HasSalt
	// reports whether the message carries it.
	Salt uint32
	// HasSalt reports whether Salt is set.
	HasSalt bool
	// Token is the login token of an RPTK (see package auth).
	Token [32]byte
	// Text is the rest of an RPTO (options), RPTG (home position), RPTRSSI
	// or RPTINTR (slot and value) or DMRT (trunking command) message. A
	// decode points it into the packet.
	Text []byte
}

// DecodeControl decodes the control message p of dialect d. It returns
// ErrCommand when p is not a control message of d, and ErrShort, ErrLength
// and ErrText as DecodeData does. Text points into p. It does not allocate.
func DecodeControl(p []byte, d Dialect) (Control, error) {
	c := Classify(p, d)
	n := len(c.tag())
	m := Control{Command: c}
	var err error
	switch c.shape(d) {
	case shapeTag:
		if len(p) != n {
			return Control{}, ErrLength
		}
	case shapeText:
		m.Text = p[n:]
	case shapeBinID:
		m.ID, err = decodeBin(p, n)
	case shapeBinIDText:
		if len(p) < n+4 {
			return Control{}, ErrShort
		}
		m.ID, err = decodeBin(p[:n+4], n)
		m.Text = p[n+4:]
	case shapeBinIDToken:
		var t binIDToken
		t, err = decodeBinIDToken(p)
		m.ID, m.Token = t.ID, t.Token
	case shapeHexID:
		m.ID, err = decodeHex(p, n)
	case shapeHexIDSalt:
		if len(p) <= 14 {
			m.ID, err = decodeHex(p, n)
			break
		}
		var t hexIDSalt
		t, err = decodeHexIDSalt(p)
		m.ID, m.Salt, m.HasSalt = t.ID, t.Salt, true
	case shapeHexIDText:
		if len(p) < n+8 {
			return Control{}, ErrShort
		}
		m.ID, err = decodeHex(p[:n+8], n)
		m.Text = p[n+8:]
	case shapeHexIDToken:
		var t hexIDToken
		t, err = decodeHexIDToken(p)
		m.ID, m.Token = t.ID, t.Token
	case shapeNone:
		return Control{}, ErrCommand
	}
	if err != nil {
		return Control{}, err
	}
	return m, nil
}

func decodeBin(p []byte, n int) (uint32, error) {
	switch n {
	case 4:
		m, err := decodeBinID4(p)
		return m.ID, err
	case 5:
		m, err := decodeBinID5(p)
		return m.ID, err
	case 6:
		m, err := decodeBinID6(p)
		return m.ID, err
	default:
		m, err := decodeBinID7(p)
		return m.ID, err
	}
}

func decodeHex(p []byte, n int) (uint32, error) {
	switch n {
	case 4:
		m, err := decodeHexID4(p)
		return m.ID, err
	case 5:
		m, err := decodeHexID5(p)
		return m.ID, err
	case 6:
		m, err := decodeHexID6(p)
		return m.ID, err
	default:
		m, err := decodeHexID7(p)
		return m.ID, err
	}
}

// AppendBinary appends the message c in dialect d to dst. HasSalt selects
// the long form of a DialectHBRP MSTACK. It returns dst unchanged and an
// error when d has no such message, or when Text is set on a message
// without text, and dmrgo.ErrNilReceiver for a nil c. It allocates only to
// grow dst.
func (c *Control) AppendBinary(dst []byte, d Dialect) ([]byte, error) {
	if c == nil {
		return dst, dmrgo.ErrNilReceiver
	}
	return c.appendBinary(dst, d)
}

func (c Control) appendBinary(dst []byte, d Dialect) ([]byte, error) {
	if !c.Command.inDialect(d) {
		return dst, errDialectCommand
	}
	t := c.Command.tag()
	sh := c.Command.shape(d)
	switch sh {
	case shapeText, shapeBinIDText, shapeHexIDText:
	case shapeNone:
		return dst, errDialectCommand
	default:
		if len(c.Text) != 0 {
			return dst, errTextLength
		}
	}
	n := len(dst)
	var err error
	switch sh {
	case shapeTag, shapeText:
		dst = append(dst, t...)
	case shapeBinID, shapeBinIDText:
		dst, err = appendBin(dst, t, c.ID)
	case shapeBinIDToken:
		m := binIDToken{ID: c.ID, Token: c.Token}
		copy(m.Tag[:], t)
		dst, err = m.AppendBinary(dst)
	case shapeHexID, shapeHexIDText:
		dst, err = appendHex(dst, t, c.ID)
	case shapeHexIDSalt:
		if !c.HasSalt {
			dst, err = appendHex(dst, t, c.ID)
			break
		}
		m := hexIDSalt{ID: c.ID, Salt: c.Salt}
		copy(m.Tag[:], t)
		dst, err = m.AppendBinary(dst)
	case shapeHexIDToken:
		m := hexIDToken{ID: c.ID, Token: c.Token}
		copy(m.Tag[:], t)
		dst, err = m.AppendBinary(dst)
	case shapeNone:
	}
	if err != nil {
		return dst[:n], err
	}
	switch sh {
	case shapeText, shapeBinIDText, shapeHexIDText:
		dst = append(dst, c.Text...)
	default:
	}
	return dst, nil
}

func appendBin(dst []byte, t string, id uint32) ([]byte, error) {
	switch len(t) {
	case 4:
		m := binID4{ID: id}
		copy(m.Tag[:], t)
		return m.AppendBinary(dst)
	case 5:
		m := binID5{ID: id}
		copy(m.Tag[:], t)
		return m.AppendBinary(dst)
	case 6:
		m := binID6{ID: id}
		copy(m.Tag[:], t)
		return m.AppendBinary(dst)
	default:
		m := binID7{ID: id}
		copy(m.Tag[:], t)
		return m.AppendBinary(dst)
	}
}

func appendHex(dst []byte, t string, id uint32) ([]byte, error) {
	switch len(t) {
	case 4:
		m := hexID4{ID: id}
		copy(m.Tag[:], t)
		return m.AppendBinary(dst)
	case 5:
		m := hexID5{ID: id}
		copy(m.Tag[:], t)
		return m.AppendBinary(dst)
	case 6:
		m := hexID6{ID: id}
		copy(m.Tag[:], t)
		return m.AppendBinary(dst)
	default:
		m := hexID7{ID: id}
		copy(m.Tag[:], t)
		return m.AppendBinary(dst)
	}
}
