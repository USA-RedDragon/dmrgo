// Command udp sends filled forms as UDP datagrams over DMR confirmed data,
// between two simulated radios on a channel with bit errors, and prints the
// size of each form encoding and the time from a form to its
// acknowledgement at rate 1/2, 3/4 and 1.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
)

func main() {
	forms := flag.Int("forms", 30, "forms per rate and bit error rate")
	seed := flag.Uint64("seed", 1, "seed of the forms and the channel")
	flag.Parse()
	if err := run(os.Stdout, *forms, *seed); err != nil {
		fmt.Fprintln(os.Stderr, "udp:", err)
		os.Exit(1)
	}
}

func rates() []enums.DataType {
	return []enums.DataType{enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1}
}

func bers() []float64 {
	return []float64{0, 0.001, 0.005, 0.01, 0.02}
}

func run(w io.Writer, forms int, seed uint64) error {
	t := serviceOrder()
	dict := trainDictionary(&t, dictSeed)
	if err := sizes(w, &t, dict, seed); err != nil {
		return err
	}
	var cells []cell
	for _, enc := range []encoding{encScreen, encFieldsDict} {
		for _, r := range rates() {
			for _, b := range bers() {
				cells = append(cells, cell{rate: r, ber: b, enc: enc, forms: forms, seed: seed})
			}
		}
	}
	results := make([]cellResult, len(cells))
	errs := make([]error, len(cells))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for i := range cells {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			results[i], errs[i] = runCell(cells[i], &t, dict)
			<-sem
		}()
	}
	wg.Wait()
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Time from a form to its acknowledgement, %d forms each, one at a time:\n\n", forms)
	fmt.Fprintln(w, "| encoding | rate | BER | acked | mean s | p95 s | max s | bursts/form | DMR tries/form | app resends | copies dropped |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|---|---|")
	for i, r := range results {
		if errs[i] != nil {
			return fmt.Errorf("%v %v BER %g: %w", r.enc, r.rate, r.ber, errs[i])
		}
		fmt.Fprintf(w, "| %v | %v | %g | %d/%d | %s | %s | %s | %.1f | %.2f | %d | %d |\n",
			r.enc, r.rate, r.ber, r.tx.acked, r.tx.submitted,
			seconds(r.tx.latencies, mean(r.tx.latencies)), seconds(r.tx.latencies, percentile(r.tx.latencies, 0.95)),
			seconds(r.tx.latencies, percentile(r.tx.latencies, 1)),
			float64(r.air.bursts)/float64(forms), float64(r.link.tries)/float64(max(r.link.datagrams, 1)),
			r.tx.resends, r.rx.copies)
	}
	return nil
}

func seconds(all []time.Duration, d time.Duration) string {
	if len(all) == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", d.Seconds())
}

func sizes(w io.Writer, t *template, dict []byte, seed uint64) error {
	c, err := newCodec(dictID, dict)
	if err != nil {
		return err
	}
	defer c.close()
	rng := newRand(seed + 7)
	const n = 200
	var total, lo, hi [numEncodings]int
	var buf []byte
	for i := range n {
		f := randomForm(rng, t)
		for e := range numEncodings {
			buf = c.encode(buf[:0], e, &f)
			got, err := c.decode(e, t, buf)
			if err != nil {
				return fmt.Errorf("%v: %w", e, err)
			}
			for k := range got.values {
				if got.values[k] != f.values[k] {
					return fmt.Errorf("%v: field %s does not round trip", e, t.fields[k].label)
				}
			}
			k := len(buf)
			total[e] += k
			if i == 0 || k < lo[e] {
				lo[e] = k
			}
			hi[e] = max(hi[e], k)
		}
	}
	hdr, err := ip.AppendCompressed(nil, &ip.Datagram{SourcePort: appPort, DestinationPort: appPort})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Octets of the form body, %d forms, with a %d-octet dictionary. Each datagram adds a 5-octet form header and a %d-octet compressed UDP/IP header:\n\n", n, len(dict), len(hdr))
	fmt.Fprintln(w, "| encoding | mean | min | max |")
	fmt.Fprintln(w, "|---|---|---|---|")
	for e := range numEncodings {
		fmt.Fprintf(w, "| %v | %.1f | %d | %d |\n", e, float64(total[e])/n, lo[e], hi[e])
	}
	return nil
}
