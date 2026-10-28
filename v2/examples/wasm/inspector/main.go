//go:build !js

package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	format := flag.String("format", "auto", "input form: auto, hex, pcap or binary")
	proto := flag.String("protocol", "auto", "protocol: auto, ipsc, dmrd, hytera, mmdvm, hdap or burst")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: inspector [-format f] [-protocol p] file")
		os.Exit(2)
	}
	in, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "inspector:", err)
		os.Exit(1)
	}
	fmt.Println(InspectJSON(in, Options{Format: *format, Protocol: *proto}))
}
