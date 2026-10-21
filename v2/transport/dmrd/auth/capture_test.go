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
