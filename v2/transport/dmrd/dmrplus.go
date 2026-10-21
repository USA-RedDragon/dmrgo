package dmrd

// OptionState is whether a DMR+ startup option is in the options text.
type OptionState uint8

// The OptionState values.
const (
	// OptionAbsent leaves the setting of the master as it is.
	OptionAbsent OptionState = iota
	// OptionClear is an option with an empty value: it clears the setting of
	// the master.
	OptionClear
	// OptionSet is an option with a value: it overrides the master.
	OptionSet
)

// OptionValue is one DMR+ startup option.
type OptionValue struct {
	// State is whether the option is in the text.
	State OptionState
	// Value is the number of an OptionSet option.
	Value uint32
}

// DMRPlusOptions is the DMR+ startup options text that a repeater sends in
// Control.Text of an RPTO, for example
// "StartRef=4013;RelinkTime=15;UserLink=1;TS1_1=262;TS1_2=1;".
// (MMDVMHost DMRplus_startup_options.md, DG9VH 2016-11-11)
type DMRPlusOptions struct {
	// StartRef is the default reflector of slot 2.
	StartRef OptionValue
	// RelinkTime is the time in minutes after which the repeater falls back
	// to StartRef when there is no local traffic.
	RelinkTime OptionValue
	// UserLink is 1 when users may link to other reflectors, 0 when not.
	UserLink OptionValue
	// TS1 holds the up to 5 static talkgroups of slot 1, TS1_1 to TS1_5.
	TS1 [5]OptionValue
}

// ParseDMRPlusOptions parses a DMR+ startup options text: Key=Value pairs
// separated by semicolons, with spaces around them ignored. It skips pairs
// with an unknown key or without "=", and returns ErrText, with the options
// parsed so far, when a value is not a decimal number below 2^32. It does
// not allocate.
func ParseDMRPlusOptions(text []byte) (DMRPlusOptions, error) {
	var o DMRPlusOptions
	for len(text) > 0 {
		end := 0
		for end < len(text) && text[end] != ';' {
			end++
		}
		pair := text[:end]
		if end < len(text) {
			end++
		}
		text = text[end:]
		eq := 0
		for eq < len(pair) && pair[eq] != '=' {
			eq++
		}
		if eq == len(pair) {
			continue
		}
		i := optionIndex(trimSpace(pair[:eq]))
		if i < 0 {
			continue
		}
		v := OptionValue{State: OptionClear}
		val := trimSpace(pair[eq+1:])
		if len(val) > 0 {
			var n uint64
			for _, c := range val {
				if c < '0' || c > '9' {
					return o, ErrText
				}
				n = n*10 + uint64(c-'0')
				if n > 1<<32-1 {
					return o, ErrText
				}
			}
			v = OptionValue{State: OptionSet, Value: uint32(n)}
		}
		switch i {
		case 0:
			o.StartRef = v
		case 1:
			o.RelinkTime = v
		case 2:
			o.UserLink = v
		default:
			o.TS1[i-3] = v
		}
	}
	return o, nil
}

func optionIndex(key []byte) int {
	switch {
	case equal(key, "StartRef"):
		return 0
	case equal(key, "RelinkTime"):
		return 1
	case equal(key, "UserLink"):
		return 2
	case len(key) == 5 && hasTag(key, "TS1_") && key[4] >= '1' && key[4] <= '5':
		return 3 + int(key[4]-'1')
	}
	return -1
}

// AppendText appends the options that are not OptionAbsent to dst as a DMR+
// startup options text, each followed by a semicolon, in the order of the
// fields. It allocates only to grow dst.
func (o *DMRPlusOptions) AppendText(dst []byte) []byte {
	if o == nil {
		return dst
	}

	dst = appendOption(dst, "StartRef", o.StartRef)
	dst = appendOption(dst, "RelinkTime", o.RelinkTime)
	dst = appendOption(dst, "UserLink", o.UserLink)
	for i := range o.TS1 {
		if o.TS1[i].State == OptionAbsent {
			continue
		}
		dst = append(dst, "TS1_"...)
		dst = append(dst, byte('1'+i))
		dst = appendValue(dst, o.TS1[i])
	}
	return dst
}

func appendOption(dst []byte, key string, v OptionValue) []byte {
	if v.State == OptionAbsent {
		return dst
	}
	dst = append(dst, key...)
	return appendValue(dst, v)
}

func appendValue(dst []byte, v OptionValue) []byte {
	dst = append(dst, '=')
	if v.State == OptionSet {
		var digits [10]byte
		i := len(digits)
		n := v.Value
		for {
			i--
			digits[i] = byte('0' + n%10)
			n /= 10
			if n == 0 {
				break
			}
		}
		dst = append(dst, digits[i:]...)
	}
	return append(dst, ';')
}

func trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t') {
		b = b[1:]
	}
	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\t') {
		b = b[:len(b)-1]
	}
	return b
}

func equal(b []byte, s string) bool {
	return len(b) == len(s) && hasTag(b, s)
}
