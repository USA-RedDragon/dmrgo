package auth_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/auth"
)

// ExampleToken computes the login token for the salt of an RPTACK and
// checks the RPTK that DMRGateway sent with the password passw0rd.
func ExampleToken() {
	ack, _ := hex.DecodeString("52505441434bfcb17c08")
	rptk, _ := hex.DecodeString("5250544b003010afc95b349a634da769f56cab69f361bc30c6e5ea9a127fd973ef40a9f6a329dcfb")
	a, _ := dmrd.DecodeControl(ack, dmrd.DialectMMDVM)
	k, _ := dmrd.DecodeControl(rptk, dmrd.DialectMMDVM)

	token, err := auth.Token(dmrd.DialectMMDVM, a.ID, []byte("passw0rd"))
	fmt.Printf("%x %v\n", token, err)
	fmt.Println(token == k.Token)
	fmt.Println(auth.VerifyToken(dmrd.DialectMMDVM, a.ID, []byte("passw0rd"), k.Token[:]))
	// Output:
	// c95b349a634da769f56cab69f361bc30c6e5ea9a127fd973ef40a9f6a329dcfb <nil>
	// true
	// true
}
