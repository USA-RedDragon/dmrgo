package sms

// IPText is the payload of an ETSI text message: UDP port 5016, the text in
// UTF-16BE (ETSI TS 102 361-3 clause 5.7.1).
//
//dmr:stream
type IPText struct {
	// Text is the text in UTF-16BE.
	Text []byte `dmr:"rest,utf16be"`
}

// AnyToneText is the payload of the text that AnyTone radios send to UDP
// port 5016 in their DMR Standard mode: the octets 00 0d 00 0a, then the
// text in UTF-16LE and a NUL character.
//
//dmr:stream
type AnyToneText struct {
	// Prefix is the fixed prefix 0x000D000A.
	Prefix uint32 `dmr:"u32,const:0x000D000A"`
	// Text is the text in UTF-16LE, then a NUL character.
	Text []byte `dmr:"rest,utf16le,nul"`
}
