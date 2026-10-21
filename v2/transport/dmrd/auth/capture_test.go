package auth_test

import (
	"bufio"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/auth"
)

func TestTokenOfDMRGatewayCapture(t *testing.T) {
	t.Parallel()

	f, err := os.Open("../testdata/dmrgateway-master.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var salt uint32
	checked := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		p, err := hex.DecodeString(strings.Fields(sc.Text())[3])
		if err != nil {
			t.Fatal(err)
		}
		c, err := dmrd.DecodeControl(p, dmrd.DialectMMDVM)
		if err != nil {
			continue
		}
		switch {
		case c.Command == dmrd.CommandRPTACK && salt == 0:
			salt = c.ID
		case c.Command == dmrd.CommandRPTK:
			want, err := auth.Token(dmrd.DialectMMDVM, salt, []byte("passw0rd"))
			if err != nil || want != c.Token {
				t.Fatalf("token %x, DMRGateway sent %x (%v)", want, c.Token, err)
			}
			if !auth.VerifyToken(dmrd.DialectMMDVM, salt, []byte("passw0rd"), c.Token[:]) {
				t.Fatal("VerifyToken rejects the token of DMRGateway")
			}
			if auth.VerifyToken(dmrd.DialectMMDVM, salt, []byte("passw0rD"), c.Token[:]) {
				t.Fatal("VerifyToken accepts a wrong password")
			}
			checked = true
		}
	}
	if !checked {
		t.Fatal("no RPTK in the capture")
	}
}

func TestHBlink3OpenBridge(t *testing.T) {
	t.Parallel()

	f, err := os.Open("../testdata/hblink3-openbridge.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b := auth.NewBridge([]byte("passw0rd-longer-than-twenty"))
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		p, err := hex.DecodeString(strings.Fields(sc.Text())[1])
		if err != nil {
			t.Fatal(err)
		}
		if !b.Verify(p) {
			t.Fatalf("%x does not verify", p)
		}
		d, err := dmrd.DecodeBridge(p)
		if err != nil || d.Version != 1 || d.Data.Repeater != 3129100 || d.Data.Destination != 91 {
			t.Fatalf("%x: %+v %v", p, d, err)
		}
		q := append([]byte(nil), p...)
		q[20] ^= 1
		if b.Verify(q) {
			t.Fatalf("%x verifies with a changed burst", q)
		}
		n++
	}
	if n != 8 {
		t.Fatalf("%d packets", n)
	}
}
