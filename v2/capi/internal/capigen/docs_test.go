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
		"/* See voice.Receiver.Scratch, sms.Bad, other.Name and Header.UAB. */": "/* See dmrgo_voice_receiver_set_scratch, sms.Bad, other.Name and Header.UAB. */",
		"/* As net.UDPAddr holds them. */":                                      "/* As a UDP socket address holds them. */",
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
		{"The P_HEAD, or nil. When nil, none.", []string{"bool has_proprietary", "dmrgo_pdu_proprietary_header_t proprietary"}, "The P_HEAD, or has_proprietary false. When has_proprietary false, none."},
		{"A nil return keeps LC.", []string{"dmrgo_x_fn_t embedded_lc"}, "A false *out_ok keeps LC."},
	} {
		if got := nilC(c.d, c.decls); got != c.want {
			t.Errorf("nilC(%q) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestNilWords(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"Encode returns 3 and a nil error.":          "Encode returns 3 and DMRGO_OK.",
		"Err returns X when set, and nil otherwise.": "Err returns X when set, and DMRGO_OK otherwise.",
		"For a nil cfg or a non-nil rd. Nil is 0.":   "For a NULL cfg or a non-NULL rd. NULL is 0.",
		"A Nilsson and vanilla.":                     "A Nilsson and vanilla.",
	} {
		if got := nilWords(in); got != want {
			t.Errorf("nilWords(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChainRefs(t *testing.T) {
	t.Parallel()

	refs := map[string]string{
		"trunk.Output.Grant":    "dmrgo_trunk_output_t.grant",
		"trunk.Grant.Source":    "dmrgo_trunk_grant_t.source",
		"ipsc.User.Burst":       "dmrgo_ipsc_user_t.burst",
		"ipsc.BurstType.Type":   "dmrgo_ipsc_burst_type_type",
		"layer2.Burst.SlotType": "dmrgo_layer2_burst_set_slot_type",
		"pdu.SlotType.DataType": "dmrgo_pdu_slot_type_t.data_type",
	}
	chainRefs(refs, map[string][][]string{
		"trunk.Output.Grant.Source":      {{"trunk.Output.Grant", "trunk.Grant.Source"}},
		"ipsc.User.Burst.Type":           {{"ipsc.User.Burst", "ipsc.BurstType.Type"}},
		"layer2.Burst.SlotType.DataType": {{"layer2.Burst.SlotType", "pdu.SlotType.DataType"}, {"pdu.SlotType.DataType"}},
		"trunk.Output.Grant.Missing":     {{"trunk.Output.Grant", "trunk.Grant.Missing"}},
	})
	for name, want := range map[string]string{
		"trunk.Output.Grant.Source":      "dmrgo_trunk_output_t.grant.source",
		"ipsc.User.Burst.Type":           "dmrgo_ipsc_burst_type_type on dmrgo_ipsc_user_t.burst",
		"layer2.Burst.SlotType.DataType": "dmrgo_pdu_slot_type_t.data_type",
		"trunk.Output.Grant.Missing":     "",
	} {
		if got := refs[name]; got != want {
			t.Errorf("refs[%q] = %q, want %q", name, got, want)
		}
	}
	in := "/* From trunk.Output.Grant.Source or trunk.Output.Grant.Missing. */"
	want := "/* From dmrgo_trunk_output_t.grant.source or dmrgo_trunk_output_t.grant.Missing. */"
	if got := cDocs(in, refs); got != want {
		t.Errorf("cDocs(%q) = %q, want %q", in, got, want)
	}
}
