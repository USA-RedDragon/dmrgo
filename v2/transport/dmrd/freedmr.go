package dmrd

import "github.com/USA-RedDragon/dmrgo/v2/enums"

// MaxFreeDMROptionsLen is the longest options text that FreeDMR reads. It
// ignores a longer one.
const MaxFreeDMROptionsLen = 255

// FreeDMROptions is the FreeDMR options text that a repeater or hotspot
// sends in Control.Text of an RPTO, for example
// "TS1=91,2350;TS2=;DIAL=0;VOICE=1;TIMER=10;". Each field names the keys
// that set it, the first one first. An option is OptionAbsent when no key
// sets it, when its value is empty, and when its value is not valid: in
// each case FreeDMR keeps the setting of the master. DialTS1 and DialTS2
// are OptionClear for an empty value: FreeDMR then links no reflector. The
// static talkgroups are read by AppendFreeDMRStatic.
//
// HBlink3 reads no options: a master acknowledges the RPTO, and a peer
// sends "Type=HBlink;" before the options of its configuration, which
// ParseFreeDMROptions skips as an unknown key.
//
// (FreeDMR 546165eb bridge_master.py options_config and hblink.py RPTO;
// HBlink3 1db2730 hblink.py RPTO and config.py OPTIONS)
type FreeDMROptions struct {
	// DialTS1 is the reflector that slot 1 links to: default_dial_ts1,
	// DIAL1 or DEFAULT_DIAL_TS1.
	DialTS1 OptionValue
	// DialTS2 is the reflector that slot 2 links to: default_dial_ts2,
	// DIAL2, DEFAULT_DIAL_TS2, DIAL, DEFAULT_REFLECTOR or the DMR+
	// StartRef.
	DialTS2 OptionValue
	// Timer is the time in minutes after which a talkgroup or reflector
	// that a user linked drops: the DMR+ RelinkTime, TIMER or
	// DEFAULT_UA_TIMER. 0 means no limit, here and in the other timers.
	Timer OptionValue
	// TS1Timer is the timer of slot 1: TS1Timer or TS1_TIMER.
	TS1Timer OptionValue
	// TS2Timer is the timer of slot 2: TS2Timer or TS2_TIMER.
	TS2Timer OptionValue
	// TGTimer is the timer of talkgroups: TGTimer or TG_TIMER.
	TGTimer OptionValue
	// DialTimer is the timer of reflectors: DialTimer or DIAL_TIMER.
	DialTimer OptionValue
	// TS1TGTimer is the timer of talkgroups on slot 1: TGTO1 or
	// TS1_TG_TIMER.
	TS1TGTimer OptionValue
	// TS2TGTimer is the timer of talkgroups on slot 2: TGTO2 or
	// TS2_TG_TIMER.
	TS2TGTimer OptionValue
	// TS1DialTimer is the timer of reflectors on slot 1: DIALTO1 or
	// TS1_DIAL_TIMER.
	TS1DialTimer OptionValue
	// TS2DialTimer is the timer of reflectors on slot 2: DIALTO2 or
	// TS2_DIAL_TIMER.
	TS2DialTimer OptionValue
	// Voice is 1 when the master sends voice announcements, 0 when not:
	// IDENT or VOICE.
	Voice OptionValue
	// IdentTG is the talkgroup of the voice ident: IDENTTG, VOICETG or
	// OVERRIDE_IDENT_TG.
	IdentTG OptionValue
	// Single is 1 for single mode, in which a slot carries one talkgroup
	// at a time, 0 when not: SINGLE.
	Single OptionValue
	// DialATG is 1 when users may link reflectors (dial-a-TG), 0 when not:
	// DIALTG or DIAL_A_TG.
	DialATG OptionValue
	// Dynamic is 1 when users may link talkgroups by calling them, 0 when
	// not: DYNAMIC or DYNAMIC_TG_ROUTING.
	Dynamic OptionValue
	// DirectDialSlot is the slot of network direct dial, 0, 1 or 2:
	// NDD_SLOT or NETWORK_DIRECT_DIAL_SLOT.
	DirectDialSlot OptionValue
	// Key is the value of KEY, which FreeDMR compares with the key of the
	// first options it accepted. It is a view of the text; empty means no
	// key.
	Key []byte
	// Lang is the value of LANG, the language of the voice announcements,
	// as a view of the text.
	Lang []byte
}

// ParseFreeDMROptions parses a FreeDMR options text as FreeDMR does:
// Key=Value pairs separated by semicolons, keys in the exact case, the last
// pair of a key wins, and pairs with no "=" or more than one are skipped.
// It ignores trailing zero octets, quote characters and octets that are not
// ASCII, and the white space around a number. It returns ErrText and no
// options for a text longer than MaxFreeDMROptionsLen, and ErrText with
// all the valid options when a value is not a number, or is out of range
// for a 0 or 1 option or DirectDialSlot. Key and Lang are views of text,
// in which quote characters and other octets that are not ASCII stay. It
// does not allocate.
func ParseFreeDMROptions(text []byte) (FreeDMROptions, error) {
	var o FreeDMROptions
	text, ok := freeDMRText(text)
	if !ok {
		return o, ErrText
	}
	bad := false
	o.DialTS1 = freeDMRDial(text, &bad, "default_dial_ts1", "DIAL1", "DEFAULT_DIAL_TS1", "", "", "")
	o.DialTS2 = freeDMRDial(text, &bad, "default_dial_ts2", "DIAL2", "DEFAULT_DIAL_TS2", "DIAL", "DEFAULT_REFLECTOR", "StartRef")
	o.Timer = freeDMRNumber(text, &bad, maxOptionValue, "RelinkTime", "TIMER", "DEFAULT_UA_TIMER")
	o.TS1Timer = freeDMRNumber(text, &bad, maxOptionValue, "TS1Timer", "TS1_TIMER", "")
	o.TS2Timer = freeDMRNumber(text, &bad, maxOptionValue, "TS2Timer", "TS2_TIMER", "")
	o.TGTimer = freeDMRNumber(text, &bad, maxOptionValue, "TGTimer", "TG_TIMER", "")
	o.DialTimer = freeDMRNumber(text, &bad, maxOptionValue, "DialTimer", "DIAL_TIMER", "")
	o.TS1TGTimer = freeDMRNumber(text, &bad, maxOptionValue, "TGTO1", "TS1_TG_TIMER", "")
	o.TS2TGTimer = freeDMRNumber(text, &bad, maxOptionValue, "TGTO2", "TS2_TG_TIMER", "")
	o.TS1DialTimer = freeDMRNumber(text, &bad, maxOptionValue, "DIALTO1", "TS1_DIAL_TIMER", "")
	o.TS2DialTimer = freeDMRNumber(text, &bad, maxOptionValue, "DIALTO2", "TS2_DIAL_TIMER", "")
	o.Voice = freeDMRNumber(text, &bad, 1, "IDENT", "VOICE", "")
	o.IdentTG = freeDMRNumber(text, &bad, maxOptionValue, "IDENTTG", "VOICETG", "OVERRIDE_IDENT_TG")
	o.Single = freeDMRNumber(text, &bad, 1, "SINGLE", "", "")
	o.DialATG = freeDMRNumber(text, &bad, 1, "DIALTG", "DIAL_A_TG", "")
	o.Dynamic = freeDMRNumber(text, &bad, 1, "DYNAMIC", "DYNAMIC_TG_ROUTING", "")
	o.DirectDialSlot = freeDMRNumber(text, &bad, 2, "NDD_SLOT", "NETWORK_DIRECT_DIAL_SLOT", "")
	if s, e, ok := freeDMRValue(text, "KEY", 0); ok {
		o.Key = text[s:e]
	}
	if s, e, ok := freeDMRValue(text, "LANG", 0); ok {
		o.Lang = text[s:e]
	}
	if bad {
		return o, ErrText
	}
	return o, nil
}

// AppendFreeDMRStatic appends to dst the static talkgroups of slot s in a
// FreeDMR options text, as FreeDMR reads them: the DMR+ keys TS1_1 to TS1_9
// (TS2_1 to TS2_9 for slot 2) when TS1_1 is there, else the comma list of
// TS1 or TS1_STATIC. It skips an entry that is not a number below 2^24,
// and ignores trailing zero octets, white space, quote characters and
// octets that are not ASCII.
// FreeDMR also skips 0 to 9, 9990 to 9999 and 16777215. It appends nothing
// for a slot other than enums.Slot1 and enums.Slot2 or a text longer than
// MaxFreeDMROptionsLen, and allocates only to grow dst.
func AppendFreeDMRStatic(dst []uint32, text []byte, s enums.Slot) []uint32 {
	text, ok := freeDMRText(text)
	if !ok || s != enums.Slot1 && s != enums.Slot2 {
		return dst
	}
	short, long, numbered := "TS1", "TS1_STATIC", "TS1_"
	if s == enums.Slot2 {
		short, long, numbered = "TS2", "TS2_STATIC", "TS2_"
	}
	if vs, ve, ok := freeDMRValue(text, numbered, '1'); ok {
		dst = appendTalkgroups(dst, text, vs, ve)
		for d := byte('2'); d <= '9'; d++ {
			if vs, ve, ok := freeDMRValue(text, numbered, d); ok {
				dst = appendTalkgroups(dst, text, vs, ve)
			}
		}
		return dst
	}
	vs, ve, ok := freeDMRValue(text, short, 0)
	if !ok {
		vs, ve, ok = freeDMRValue(text, long, 0)
	}
	if ok {
		dst = appendTalkgroups(dst, text, vs, ve)
	}
	return dst
}

const maxOptionValue = 1<<32 - 1

func freeDMRText(text []byte) ([]byte, bool) {
	if len(text) > MaxFreeDMROptionsLen {
		return nil, false
	}
	n := len(text)
	for n > 0 && text[n-1] == 0 {
		n--
	}
	return text[:n], true
}

func freeDMRDial(text []byte, bad *bool, k1, k2, k3, k4, k5, k6 string) OptionValue {
	s, e, ok := freeDMRFirst(text, k1, k2, k3)
	if !ok {
		s, e, ok = freeDMRFirst(text, k4, k5, k6)
	}
	if !ok {
		return OptionValue{}
	}
	v, state := parseFreeDMRNumber(text, s, e, maxOptionValue)
	switch state {
	case numberEmpty:
		return OptionValue{State: OptionClear}
	case numberBad:
		*bad = true
		return OptionValue{}
	case numberOK:
	}
	return OptionValue{State: OptionSet, Value: v}
}

func freeDMRNumber(text []byte, bad *bool, limit uint64, k1, k2, k3 string) OptionValue {
	s, e, ok := freeDMRFirst(text, k1, k2, k3)
	if !ok {
		return OptionValue{}
	}
	v, state := parseFreeDMRNumber(text, s, e, limit)
	switch state {
	case numberEmpty:
		return OptionValue{}
	case numberBad:
		*bad = true
		return OptionValue{}
	case numberOK:
	}
	return OptionValue{State: OptionSet, Value: v}
}

func freeDMRFirst(text []byte, k1, k2, k3 string) (s, e int, ok bool) {
	for _, k := range [3]string{k1, k2, k3} {
		if k == "" {
			continue
		}
		if s, e, ok = freeDMRValue(text, k, 0); ok {
			return s, e, true
		}
	}
	return 0, 0, false
}

func freeDMRValue(text []byte, key string, suffix byte) (vs, ve int, found bool) {
	start := 0
	for start <= len(text) {
		end := start
		eq, n := 0, 0
		for end < len(text) && text[end] != ';' {
			if text[end] == '=' {
				if n == 0 {
					eq = end
				}
				n++
			}
			end++
		}
		if n == 1 && freeDMRKey(text, start, eq, key, suffix) {
			vs, ve, found = eq+1, end, true
		}
		start = end + 1
	}
	return vs, ve, found
}

func freeDMRKey(text []byte, s, e int, key string, suffix byte) bool {
	j := 0
	for i := s; i < e; i++ {
		c := text[i]
		if freeDMRDropped(c) {
			continue
		}
		switch {
		case j < len(key) && c == key[j]:
		case j == len(key) && suffix != 0 && c == suffix:
		default:
			return false
		}
		j++
	}
	if suffix != 0 {
		return j == len(key)+1
	}
	return j == len(key)
}

func freeDMRDropped(c byte) bool {
	return c == '\'' || c == '"' || c >= 0x80
}

func pySpace(c byte) bool {
	return c == ' ' || c >= '\t' && c <= '\r' || c >= 0x1C && c <= 0x1F
}

type numberState uint8

const (
	numberOK numberState = iota
	numberEmpty
	numberBad
)

func parseFreeDMRNumber(text []byte, s, e int, limit uint64) (uint32, numberState) {
	var n uint64
	digits, trailing := 0, false
	for i := s; i < e; i++ {
		c := text[i]
		switch {
		case freeDMRDropped(c):
		case pySpace(c):
			trailing = digits > 0
		case c >= '0' && c <= '9' && !trailing:
			digits++
			n = n*10 + uint64(c-'0')
			if n > limit {
				return 0, numberBad
			}
		default:
			return 0, numberBad
		}
	}
	if digits == 0 {
		return 0, numberEmpty
	}
	return uint32(n), numberOK
}

func appendTalkgroups(dst []uint32, text []byte, s, e int) []uint32 {
	var n uint32
	digits, ok := 0, true
	for i := s; i <= e; i++ {
		if i == e || text[i] == ',' {
			if digits > 0 && ok {
				dst = append(dst, n)
			}
			n, digits, ok = 0, 0, true
			continue
		}
		c := text[i]
		switch {
		case freeDMRDropped(c), pySpace(c):
		case c >= '0' && c <= '9':
			digits++
			if ok {
				n = n*10 + uint32(c-'0')
				ok = n < 1<<24
			}
		default:
			ok = false
		}
	}
	return dst
}
