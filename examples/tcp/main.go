// Command tcp runs a TN3270 session over TCP/IPv4 over DMR confirmed data,
// between two simulated radios on a channel with bit errors, with the TCP of
// gVisor netstack at each end, and prints the time of each screen.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func main() {
	cycles := flag.Int("cycles", 5, "form and list cycles per session")
	seed := flag.Uint64("seed", 1, "seed of the screens and the channel")
	mtu := flag.Uint("mtu", 576, "MTU of the DMR link")
	flag.Parse()
	run(os.Stdout, *cycles, *seed, uint32(min(*mtu, 1266))) //nolint:gosec // bounded
}

func rates() []enums.DataType {
	return []enums.DataType{enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1}
}

func bers() []float64 {
	return []float64{0, 0.001, 0.005, 0.01, 0.02}
}

func run(w io.Writer, cycles int, seed uint64, mtu uint32) {
	cells := make([]cell, 0, 4*len(rates())*len(bers()))
	for _, duplex := range []bool{false, true} {
		for _, win := range []int{1, 4} {
			for _, r := range rates() {
				for _, b := range bers() {
					cells = append(cells, cell{duplex: duplex, rate: r, ber: b, window: win, mtu: mtu, cycles: cycles, seed: seed})
				}
			}
		}
	}
	results := make([]cellResult, len(cells))
	errs := make([]error, len(cells))
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(runtime.GOMAXPROCS(0)/2, 1))
	for i := range cells {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			results[i], errs[i] = runCell(cells[i])
			<-sem
		}()
	}
	wg.Wait()
	rng := newRand(seed + 9)
	fmt.Fprintf(w, "TN3270 over TCP over DMR, MTU %d, %d cycles of form submit and back. Records: negotiation %d octets in 6 records, form screen %d, form input %d, list screen %d, PF3 %d:\n\n",
		mtu, cycles, negotiationOctets(), len(formScreen()), len(formInput(rng)), len(listScreen(rng)), len(pf3()))
	fmt.Fprintln(w, "| channel | window | rate | BER | setup s | submit mean s | submit p95 s | back mean s | TCP segments | TCP retransmits | RTO timeouts | DMR packets | bursts |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|---|---|---|---|")
	for i, r := range results {
		if errs[i] != nil {
			fmt.Fprintf(w, "| %s | %d | %v | %g | %v | | | | | | | | |\n", mode(r.duplex), r.window, r.rate, r.ber, errs[i])
			continue
		}
		seg := r.tcp[0].segments + r.tcp[1].segments
		re := r.tcp[0].retransmits + r.tcp[1].retransmits
		to := r.tcp[0].timeouts + r.tcp[1].timeouts
		fmt.Fprintf(w, "| %s | %d | %v | %g | %.1f | %.1f | %.1f | %.1f | %d | %d | %d | %d | %d |\n",
			mode(r.duplex), r.window, r.rate, r.ber, r.setup.Seconds(), mean(r.submit).Seconds(), percentile(r.submit, 0.95).Seconds(),
			mean(r.back).Seconds(), seg, re, to, r.links[0].packets+r.links[1].packets, r.air.bursts)
	}
}

func mode(duplex bool) string {
	if duplex {
		return "duplex"
	}
	return "simplex"
}

func negotiationOctets() int {
	n := 0
	for _, b := range negotiation() {
		n += len(b)
	}
	return n
}
