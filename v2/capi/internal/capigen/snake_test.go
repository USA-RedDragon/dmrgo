package capigen

import (
	"strings"
	"testing"
)

func TestSnake(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"CSBKOpcode":                    "csbk_opcode",
		"DecodeFullLinkControlInto":     "decode_full_link_control_into",
		"FLCGPSInfo":                    "flc_gps_info",
		"MBCLastBlockCRCOK":             "mbc_last_block_crc_ok",
		"ConfirmedCRC9OK":               "confirmed_crc9_ok",
		"UDPIPv4CompressedHeader":       "udp_ipv4_compressed_header",
		"ShortLCCSysParms":              "short_lc_c_sys_parms",
		"ShortLCPSysParms":              "short_lc_p_sys_parms",
		"AnnWDTSCCParms":                "ann_wd_tscc_parms",
		"TS1ActivityID":                 "ts1_activity_id",
		"Rate12Data":                    "rate12_data",
		"DecodeAMBE72":                  "decode_ambe72",
		"Opcode0x38Hint":                "opcode0x38_hint",
		"MMDVMHostSeal":                 "mmdvm_host_seal",
		"ErrTalkerAliasInvalidUTF8":     "err_talker_alias_invalid_utf8",
		"VoiceTransmitter":              "voice_transmitter",
		"TSCCOutputKind":                "tscc_output_kind",
		"ID":                            "id",
		"DecodeUDPIPv4CompressedHeader": "decode_udp_ipv4_compressed_header",
		"DecodeP2P":                     "decode_p2p",
		"Circle2D":                      "circle2d",
		"Point3DAccuracy":               "point3d_accuracy",
		"RxFrequencyMHz":                "rx_frequency_mhz",
		"RxFrequencyHz":                 "rx_frequency_hz",
		"DeviationHz":                   "deviation_hz",
	} {
		if got := snake(in); got != want {
			t.Errorf("snake(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCKeyword(t *testing.T) {
	t.Parallel()

	for _, k := range []string{"class", "new", "delete", "this", "default", "signed", "user", "template", "and"} {
		if got := cKeyword(k); got != k+"_" {
			t.Errorf("cKeyword(%q) = %q", k, got)
		}
	}
	if got := cKeyword("burst"); got != "burst" {
		t.Errorf("cKeyword(burst) = %q", got)
	}
}

func TestZeroReady(t *testing.T) {
	t.Parallel()

	for doc, want := range map[string]bool{
		"CACH makes the CACH field. The zero CACH is ready: its first Next starts a Short LC.": true,
		"Decoder decodes. The zero value is ready to use.":                                     true,
		"BERCalculator counts. The zero value is an empty calculator.":                         true,
		"ChannelPlan maps. The zero ChannelPlan is empty and ready to use.":                    true,
		"Receiver receives. Make it with NewReceiver.":                                         false,
		"Framer frames. Make one with New: a zero Framer has no buffer and drops every bit.":   false,
		"Assembler assembles. The zero value has no storage for blocks.":                       false,
		"Codec codes. The zero Codec is closed.":                                               false,
	} {
		name, _, _ := strings.Cut(doc, " ")
		if got := zeroReady(doc, name); got != want {
			t.Errorf("zeroReady(%q) = %t, want %t", doc, got, want)
		}
	}
}
