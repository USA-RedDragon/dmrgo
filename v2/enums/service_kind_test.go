package enums_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestNRandWait_Table6_2(t *testing.T) {
	t.Parallel()
	want := [16]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 15, 24}
	for code, frames := range want {
		if got := enums.NRandWait(code).Frames(); got != frames {
			t.Errorf("NRandWait(%d).Frames() = %d, want %d", code, got, frames)
		}
	}
	if got := enums.NRandWait(16).Frames(); got != 0 {
		t.Errorf("NRandWait(16).Frames() = %d, want 0", got)
	}
}

func TestBackoff_Table6_3(t *testing.T) {
	t.Parallel()
	want := [16]int{0, 1, 2, 3, 4, 5, 8, 11, 15, 20, 26, 33, 41, 50, 70, 100}
	for code, frames := range want {
		if got := enums.Backoff(code).Frames(); got != frames {
			t.Errorf("Backoff(%d).Frames() = %d, want %d", code, got, frames)
		}
	}
	if got := enums.Backoff(16).Frames(); got != 0 {
		t.Errorf("Backoff(16).Frames() = %d, want 0", got)
	}
}

func TestServiceFunction_Table6_1(t *testing.T) {
	t.Parallel()
	tests := []struct {
		f    enums.ServiceFunction
		k    enums.ServiceKind
		want bool
	}{
		{enums.ServiceFunctionAll, enums.ServiceKindStatusTransport, true},
		{enums.ServiceFunctionPayload, enums.ServiceKindTalkgroupVoice, true},
		{enums.ServiceFunctionPayload, enums.ServiceKindRegistration, true},
		{enums.ServiceFunctionPayload, enums.ServiceKindIndividualShortData, false},
		{enums.ServiceFunctionNoPayload, enums.ServiceKindIndividualShortData, true},
		{enums.ServiceFunctionNoPayload, enums.ServiceKindRegistration, true},
		{enums.ServiceFunctionNoPayload, enums.ServiceKindIndividualVoice, false},
		{enums.ServiceFunctionRegistration, enums.ServiceKindRegistration, true},
		{enums.ServiceFunctionRegistration, enums.ServiceKindTalkgroupVoice, false},
		{enums.ServiceFunction(4), enums.ServiceKindRegistration, false},
	}
	for _, tt := range tests {
		if got := tt.f.Invites(tt.k); got != tt.want {
			t.Errorf("%v.Invites(%v) = %v, want %v", tt.f, tt.k, got, tt.want)
		}
	}
}

func TestServiceKind_Table7_49(t *testing.T) {
	t.Parallel()
	payload := map[enums.ServiceKind]bool{0: true, 1: true, 2: true, 3: true, 10: true, 11: true}
	for k := range enums.ServiceKind(16) {
		if got := k.NeedsPayloadChannel(); got != payload[k] {
			t.Errorf("%v.NeedsPayloadChannel() = %v", k, got)
		}
		if s := k.String(); s == "" || (k == 12) != strings.HasPrefix(s, "Reserved") {
			t.Errorf("ServiceKind(%d).String() = %q", k, s)
		}
	}
	for f := range enums.ServiceFunction(5) {
		if f.String() == "" {
			t.Errorf("ServiceFunction(%d).String() is empty", f)
		}
	}
}

func TestAckReason_Tables7_42to7_45(t *testing.T) {
	t.Parallel()
	tests := []struct {
		r      enums.AckReason
		typ    enums.AckType
		fromTS bool
		name   string
	}{
		{enums.AckReasonRegAccepted, enums.AckACK, true, "Reg_Accepted"},
		{enums.AckReasonMSAccepted, enums.AckACK, false, "MS_Accepted"},
		{enums.AckReasonMSAwayRefused, enums.AckNACK, true, "MSaway_Refused"},
		{enums.AckReasonRegDenied, enums.AckNACK, true, "Reg_Denied"},
		{enums.AckReasonRecipientRefused, enums.AckNACK, false, "Recipient_Refused"},
		{enums.AckReasonMSNotSupported, enums.AckNACK, false, "MSNot_Supported"},
		{enums.AckReasonQueuedForResource, enums.AckQACK, true, "Queued-for-resource"},
		{enums.AckReasonWait, enums.AckWACK, true, "Wait"},
	}
	for _, tt := range tests {
		if tt.r.Type() != tt.typ || tt.r.FromTS() != tt.fromTS || tt.r.String() != tt.name {
			t.Errorf("%08b: Type %v FromTS %v String %q, want %v %v %q", uint8(tt.r), tt.r.Type(), tt.r.FromTS(), tt.r.String(), tt.typ, tt.fromTS, tt.name)
		}
	}
	for r := range 256 {
		if enums.AckReason(r).String() == "" {
			t.Errorf("AckReason(%d).String() is empty", r)
		}
	}
	for at := range enums.AckType(5) {
		if at.String() == "" {
			t.Errorf("AckType(%d).String() is empty", at)
		}
	}
}
