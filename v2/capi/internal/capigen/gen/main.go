// Command gen writes the generated C API files of libdmrgo. Run it from the
// capi directory; go generate ./... in the root module runs it. With -break
// it accepts a change of the C ABI and bumps DMRGO_ABI_VERSION.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/USA-RedDragon/dmrgo/v2/capi/internal/capigen"
)

func main() {
	brk := flag.Bool("break", false, "accept a change of the C ABI and bump DMRGO_ABI_VERSION")
	flag.Parse()
	lock, err := os.ReadFile("abi.txt")
	if err != nil && !os.IsNotExist(err) {
		log.Fatal(err)
	}
	out, err := capigen.Generate(".", lock, capigen.Options{Break: *brk})
	if out != nil {
		for _, b := range out.Broken {
			fmt.Fprintln(os.Stderr, "changed:", b)
		}
	}
	if err != nil {
		log.Fatal(err)
	}
	for name, b := range out.Files() {
		if err := os.WriteFile(name, b, 0o600); err != nil {
			log.Fatal(err)
		}
	}
	for _, u := range out.Unbounded {
		fmt.Fprintln(os.Stderr, "unbounded:", u)
	}
	missing := 0
	kinds := map[string][2]int{}
	for _, e := range out.Exports {
		k := kinds[e.Kind]
		k[1]++
		if e.Err != "" {
			missing++
			fmt.Fprintf(os.Stderr, "unmapped %s %s: %s\n", e.Kind, e.Go, e.Err)
		} else {
			k[0]++
		}
		kinds[e.Kind] = k
	}
	fmt.Fprintf(os.Stderr, "%d of %d exports mapped:", len(out.Exports)-missing, len(out.Exports))
	for _, k := range []string{"type", "field", "func", "method", "const", "var"} {
		fmt.Fprintf(os.Stderr, " %s %d/%d", k, kinds[k][0], kinds[k][1])
	}
	fmt.Fprintln(os.Stderr)
}
