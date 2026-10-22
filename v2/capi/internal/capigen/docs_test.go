package capigen

import "testing"

func TestCDocs(t *testing.T) {
	t.Parallel()

	refs := cRefs([]Export{
		{Go: "layer2.NewBurst", Kind: "func", C: []string{"dmrgo_layer2_new_burst"}},
		{Go: "constants.TCallHang", Kind: "const", C: []string{"DMRGO_CONSTANTS_T_CALL_HANG"}},
		{Go: "layer2.RawBurst.VoiceSlots", Kind: "method", C: []string{"dmrgo_layer2_raw_burst_voice_slots"}},
		{Go: "pdu.Header", Kind: "type", C: []string{"dmrgo_pdu_header_t", "DMRGO_ERR_PDU_HEADER"}},
		{Go: "voice.Receiver.Scratch", Kind: "field", C: []string{"dmrgo_voice_receiver_get_scratch", "dmrgo_voice_receiver_set_scratch"}},
		{Go: "sms.Bad", Kind: "func", Err: "unmapped"},
	})
	for in, want := range map[string]string{
		"/* Go: layer2.NewBurst. NewBurst returns a new Burst. */":              "/* Go: layer2.NewBurst. NewBurst returns a new Burst. */",
		"/* Go: layer2.NewBurst. As layer2.NewBurst does. */":                   "/* Go: layer2.NewBurst. As dmrgo_layer2_new_burst does. */",
		"\t/* 0 means constants.TCallHang (3 s). */\n\tint x;":                  "\t/* 0 means DMRGO_CONSTANTS_T_CALL_HANG (3 s). */\n\tint x;",
		"/* As layer2.RawBurst.VoiceSlots returns it, in a pdu.Header. */":      "/* As dmrgo_layer2_raw_burst_voice_slots returns it, in a dmrgo_pdu_header_t. */",
		"/* See voice.Receiver.Scratch, sms.Bad, other.Name and Header.UAB. */": "/* See voice.Receiver.Scratch, sms.Bad, other.Name and Header.UAB. */",
		"/* An pdu.Header, an other.Name, Elan layer2.NewBurst. */":             "/* A dmrgo_pdu_header_t, an other.Name, Elan dmrgo_layer2_new_burst. */",
		"/* It is an pdu.Header. */":                                            "/* It is a dmrgo_pdu_header_t. */",
		"/* Go: x.Y. a\nlayer2.NewBurst. */ layer2.NewBurst":                    "/* Go: x.Y. a\ndmrgo_layer2_new_burst. */ layer2.NewBurst",
	} {
		if got := cDocs(in, refs); got != want {
			t.Errorf("cDocs(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNilC(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		d     string
		decls []string
		want  string
	}{
		{"Nil accepts every color code.", []string{"bool has_color_code", "uint8_t color_code"}, "has_color_code false accepts every color code."},
		{"It is set. Nil means it makes its own.", []string{"dmrgo_layer2_burst_t *scratch"}, "It is set. NULL means it makes its own."},
		{"With a nil or empty Hunt. Nil is none.", []string{"uint32_t *hunt", "size_t hunt_len"}, "With a nil or empty Hunt. Nil is none."},
		{"A Nilsson. Nil", []string{"bool has_x", "int x"}, "A Nilsson. Nil"},
	} {
		if got := nilC(c.d, c.decls); got != c.want {
			t.Errorf("nilC(%q) = %q, want %q", c.d, got, c.want)
		}
	}
}
