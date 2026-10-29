// Command tool is the fixture command of the docgen tests.
//
// # Flags
//
// It takes one flag.
package main

import (
	"flag"
	"fmt"
)

func main() {
	n := flag.Int("n", 3, "how many lines to print")
	flag.Parse()
	for i := 0; i < *n; i++ {
		fmt.Println(i)
	}
}
