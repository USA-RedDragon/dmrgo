// Command gen writes internal/allocgate/allapi/allapi_gen.go. Run it from the
// v2 module directory; go generate ./... in the root module runs it.
package main

import (
	"log"
	"os"

	"github.com/USA-RedDragon/dmrgo/v2/internal/allocgate/apigen"
)

func main() {
	src, err := apigen.Generate(".")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(apigen.Out, src, 0o600); err != nil {
		log.Fatal(err)
	}
}
