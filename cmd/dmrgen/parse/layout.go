package parse

import (
	"errors"
	"fmt"
	"go/token"
	"math"
	"slices"
	"strconv"
	"strings"
)

const (
	directiveFrame = "dmr:frame"
	directiveView  = "dmr:view"
	directiveMagic = "dmr:magic"
)

func parseLayoutDirective(pdu *PDUStruct, name, arg string) error {
	switch name {
	case directiveFrame:
		if pdu.Frame != nil {
			return errors.New("duplicate dmr:frame directive")
		}
		var sizes []int
		for _, s := range strings.Fields(arg) {
			n, err := strconv.Atoi(s)
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid dmr:frame size %q", s)
			}
			if len(sizes) > 0 && n <= sizes[len(sizes)-1] {
				return fmt.Errorf("dmr:frame sizes must be ascending, got %q", arg)
			}
			sizes = append(sizes, n)
		}
		pdu.Frame = &FrameDirective{Sizes: sizes}
	case directiveView:
		if pdu.View != nil {
			return errors.New("duplicate dmr:view directive")
		}
		parts := strings.Fields(arg)
		if len(parts) != 2 || !token.IsIdentifier(parts[0]) {
			return fmt.Errorf("dmr:view wants a type name and a size, got %q", arg)
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil || n <= 0 {
			return fmt.Errorf("invalid dmr:view size %q", parts[1])
		}
		pdu.View = &ViewDirective{Type: parts[0], Size: n}
	case directiveMagic:
		m, err := parseMagic(arg)
		if err != nil {
			return fmt.Errorf("dmr:magic: %w", err)
		}
		pdu.Magic = append(pdu.Magic, m)
	}
	return nil
}

func parseMagic(arg string) (MagicDirective, error) {
	var m MagicDirective
	spec, val, ok := strings.Cut(arg, "=")
	if !ok || spec == "" || val == "" {
		return m, fmt.Errorf("want S-E=\"text\" or S-E=0xHEX, got %q", arg)
	}
	s, e, err := parseBitRange(spec)
	if err != nil {
		return m, fmt.Errorf("invalid bit range %q: %w", spec, err)
	}
	if s%8 != 0 || (e+1)%8 != 0 {
		return m, fmt.Errorf("bits %d-%d are not whole octets", s, e)
	}
	n := (e + 1 - s) / 8
	switch {
	case strings.HasPrefix(val, `"`):
		t, err := strconv.Unquote(val)
		if err != nil {
			return m, fmt.Errorf("invalid text %s: %w", val, err)
		}
		m.Bytes = []byte(t)
	case strings.HasPrefix(val, "0x"), strings.HasPrefix(val, "0X"):
		h := val[2:]
		if len(h) != 2*n {
			return m, fmt.Errorf("value %s is not %d octets", val, n)
		}
		for i := 0; i < len(h); i += 2 {
			b, err := strconv.ParseUint(h[i:i+2], 16, 8)
			if err != nil {
				return m, fmt.Errorf("invalid value %s: %w", val, err)
			}
			m.Bytes = append(m.Bytes, byte(b))
		}
	default:
		return m, fmt.Errorf("value %s is not quoted text or 0x hex", val)
	}
	if len(m.Bytes) != n {
		return m, fmt.Errorf("value %s is %d octets, the range is %d", val, len(m.Bytes), n)
	}
	m.Start = s
	return m, nil
}

func validateEncoding(f *Field) error {
	if err := validateModifierKinds(f); err != nil {
		return err
	}
	return validateDigits(f)
}

func validateModifierKinds(f *Field) error {
	integer := f.Kind == FieldUint || f.Kind == FieldInt || f.Kind == FieldEnum
	octets := f.BitStart%8 == 0 && f.BitWidth%8 == 0 && len(f.ExtraBitRanges) == 0
	hexBytes := f.Encoding == EncodingHex && f.Kind == FieldPacked && octets && f.ArrayLen*16 == f.BitWidth
	switch {
	case hexBytes:
		return nil
	case f.Encoding != EncodingBits && !integer:
		return errors.New("le, dec and hex require an integer field, or hex a packed [N]byte over 2N octets")
	case f.Encoding != EncodingBits && !octets:
		return errors.New("le, dec and hex require whole octets")
	case f.Encoding != EncodingBits && f.Signed:
		return errors.New("le, dec and hex cannot be signed")
	case f.Bias != 0 && !integer:
		return errors.New("bias requires an integer field")
	case f.Bias != 0 && f.Encoding != EncodingBits && f.Encoding != EncodingLE:
		return errors.New("bias cannot be combined with dec or hex")
	case f.Bias != 0 && f.Signed:
		return errors.New("bias cannot be signed")
	case f.Swap16 && (f.Kind != FieldPacked || !octets || f.BitWidth%16 != 0):
		return errors.New("swap16 requires a packed field of an even number of whole octets")
	case f.Text && (f.Kind != FieldPacked || !octets):
		return errors.New("text requires a packed field of whole octets")
	}
	return nil
}

func validateDigits(f *Field) error {
	if f.Kind == FieldPacked {
		return nil
	}
	digits := f.BitWidth / 8
	switch f.Encoding {
	case EncodingLE:
		if f.BitWidth > 64 {
			return fmt.Errorf("le field of %d bits is wider than 64", f.BitWidth)
		}
	case EncodingDecimal:
		if digits > 19 {
			return fmt.Errorf("dec field of %d digits does not fit in 64 bits", digits)
		}
		if w := basicWidth(f.BaseType); w > 0 && w < 64 && math.Pow10(digits)-1 > float64(uint64(1)<<w-1) {
			return fmt.Errorf("type %s cannot hold %d decimal digits", f.GoType, digits)
		}
	case EncodingHex:
		if digits > 16 {
			return fmt.Errorf("hex field of %d digits does not fit in 64 bits", digits)
		}
		if w := basicWidth(f.BaseType); w > 0 && 4*digits > w {
			return fmt.Errorf("type %s cannot hold %d hex digits", f.GoType, digits)
		}
	case EncodingBits:
	}
	return nil
}

// DigitWidth returns the number of value bits of an integer field: its
// width for binary encodings and 4 bits per hex digit.
func (f *Field) DigitWidth() int {
	if f.Encoding == EncodingHex {
		return 4 * (f.BitWidth / 8)
	}
	return f.BitWidth
}

func validateLayout(pdu *PDUStruct, maxBit int) error {
	if pdu.Payload && pdu.Frame == nil {
		return errors.New("dmr:payload requires dmr:frame")
	}
	if pdu.Frame == nil && pdu.View == nil {
		if len(pdu.Magic) > 0 {
			return errors.New("dmr:magic requires dmr:frame or dmr:view")
		}
		for _, f := range pdu.Fields {
			if f.Encoding != EncodingBits || f.Bias != 0 || f.Swap16 || f.Text {
				return fmt.Errorf("field %s: le, dec, hex, bias, swap16 and text require dmr:frame or dmr:view", f.Name)
			}
		}
		return nil
	}
	if pdu.Frame != nil && pdu.View != nil {
		return errors.New("dmr:frame and dmr:view cannot be combined")
	}
	switch {
	case pdu.FEC != nil, pdu.CRC != nil, pdu.FECMask != nil:
		return errors.New("dmr:frame and dmr:view cannot have dmr:fec or dmr:crc")
	case len(pdu.Inputs) > 0, pdu.DispatchIf != nil, pdu.SubPDU, pdu.HasInputSize, len(pdu.Variants) > 0, len(pdu.Getters) > 0:
		return errors.New("dmr:frame and dmr:view cannot have dmr:input, dmr:dispatch_if, dmr:variant, dmr:get, dmr:subpdu or dmr:input_size")
	}
	var size int
	if pdu.Frame != nil {
		size = pdu.Frame.Sizes[len(pdu.Frame.Sizes)-1]
	} else {
		size = pdu.View.Size
	}
	if maxBit >= size*8 {
		return fmt.Errorf("fields extend to bit %d beyond %d octets", maxBit, size)
	}
	for _, m := range pdu.Magic {
		if m.Start+8*len(m.Bytes) > size*8 {
			return fmt.Errorf("dmr:magic at bit %d extends beyond %d octets", m.Start, size)
		}
	}
	pdu.InputSize = size * 8
	if err := checkLayoutKinds(pdu); err != nil {
		return err
	}
	if pdu.Frame == nil {
		return nil
	}
	return checkFrameSizes(pdu)
}

func checkLayoutKinds(pdu *PDUStruct) error {
	for _, f := range pdu.Fields {
		if f.Skip {
			continue
		}
		switch f.Kind {
		case FieldBool, FieldUint, FieldInt, FieldEnum, FieldRaw, FieldPacked:
		case FieldDelegate, FieldLongitude, FieldLatitude:
			return fmt.Errorf("field %s: %s fields are not supported in dmr:frame or dmr:view", f.Name, kindName(&f))
		}
		if f.Text && pdu.View == nil {
			return fmt.Errorf("field %s: text requires dmr:view", f.Name)
		}
	}
	return nil
}

func checkFrameSizes(pdu *PDUStruct) error {
	sizes := pdu.Frame.Sizes
	for _, f := range pdu.Fields {
		if f.Skip {
			continue
		}
		if f.Tail {
			if f.Kind != FieldPacked || f.BitStart%8 != 0 || f.BitEnd+1 != sizes[len(sizes)-1]*8 || len(f.ExtraBitRanges) > 0 {
				return fmt.Errorf("field %s: tail needs a packed field that starts on an octet and ends the longest frame", f.Name)
			}
			continue
		}
		for _, r := range f.Ranges() {
			for _, s := range sizes[:len(sizes)-1] {
				if r[0] < s*8 && r[1] >= s*8 {
					return fmt.Errorf("field %s crosses the end of a %d-octet frame", f.Name, s)
				}
			}
		}
	}
	for _, m := range pdu.Magic {
		if m.Start+8*len(m.Bytes) > sizes[0]*8 {
			return fmt.Errorf("dmr:magic at bit %d is not inside the shortest frame", m.Start)
		}
	}
	if len(sizes) == 1 {
		return nil
	}
	i := slices.IndexFunc(pdu.Fields, func(f Field) bool { return f.Name == "Size" })
	if i < 0 || !pdu.Fields[i].Skip || basicWidth(pdu.Fields[i].BaseType) == 0 || pdu.Fields[i].BaseType == typeBool {
		return errors.New("a dmr:frame with more than one size needs a dmr:\"-\" integer field Size")
	}
	pdu.Frame.SizeField = "Size"
	return nil
}
