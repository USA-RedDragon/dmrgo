package rf

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

const (
	recordingsPath = "testdata/rxbench_recordings.json"
	recordingRate  = 2048000
)

type recordingTruth int

const (
	truthIPSC recordingTruth = iota
	truthDMRD
)

type recordingSource struct {
	id, name, capture string
	truth             recordingTruth
	log, dir          string
	sliceSeconds      float64
}

func recordingSources() []recordingSource {
	rf, rig := "dmrgo-rfcap/runs/", "ipsc2hbrp-rig/runs/"
	s := make([]recordingSource, 0, 11)
	s = append(s, []recordingSource{
		{"rf-voice-ms", "MS voice relayed, HackRF on the uplink", rf + "voice-ms/out.cu8", truthIPSC, rf + "voice-ms/ipsc.txt", "rx", 0},
		{"rf-voice-late", "Late entry voice relayed, HackRF on the uplink", rf + "voice-late/out.cu8", truthIPSC, rf + "voice-late/ipsc.txt", "rx", 0},
		{"rf-pi", "Voice with PI header relayed, HackRF on the uplink", rf + "pi/out.cu8", truthIPSC, rf + "pi/ipsc.txt", "rx", 0},
		{"m2rf-all-1", "Mixed voice and data, network to air, run 1", rig + "m2rf-all-1/out.cu8.zst", truthIPSC, rig + "m2rf-all-1/ipsc.txt", "<brg", 0},
		{"m2rf-all-2", "Mixed voice and data, network to air, run 2", rig + "m2rf-all-2/out.cu8.zst", truthIPSC, rig + "m2rf-all-2/ipsc.txt", "<brg", 0},
		{"bs-voice-rec2", "Voice, network to air", rig + "dmrgo-bs-voice/rec2/repeater-bs-voice.cu8", truthIPSC, rig + "dmrgo-bs-voice/rec2/ipsc-packets.txt", "<brg", 0},
		{"m2rf-voice-check", "Voice, network to air", rig + "m2rf-voice-check/out.cu8.zst", truthIPSC, rig + "m2rf-voice-check/ipsc.txt", "<brg", 0},
	}...)
	for _, g := range []string{"0", "9.7", "19.7", "29.7"} {
		s = append(s, recordingSource{"gs-g" + g, "Gain sweep, RTL gain " + g + " dB", rig + "gs-rpt-1/g" + g + ".cu8", truthDMRD, rig + "gs-rpt-1/inject.txt", "", 1})
	}
	return s
}

type oneDecimal float64

func (v oneDecimal) MarshalJSON() ([]byte, error) {
	return strconv.AppendFloat(nil, float64(v), 'f', 1, 64), nil
}

type recordingRow struct {
	Name  string     `json:"name"`
	CN    oneDecimal `json:"cn"`
	Truth int        `json:"truth"`
	Exact []int      `json:"exact"`
}

func openCapture(path string) (io.ReadCloser, error) {
	if !strings.HasSuffix(path, ".zst") {
		return os.Open(path)
	}
	cmd := exec.CommandContext(context.Background(), "zstd", "-dc", "--", path)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &zstdReader{out, cmd}, nil
}

type zstdReader struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (z *zstdReader) Close() error {
	_ = z.ReadCloser.Close()
	err := z.cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) && !exit.Exited() {
		return nil
	}
	return err
}

func eachChunk(path string, n int, f func([]byte)) error {
	r, err := openCapture(path)
	if err != nil {
		return err
	}
	buf := make([]byte, n)
	for {
		k, err := io.ReadFull(r, buf)
		if k > 0 {
			f(buf[:k])
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			_ = r.Close()
			return err
		}
	}
	return r.Close()
}

func receiveRecording(path string, tier Tier) ([]uint64, error) {
	var got []uint64
	on := func(bs []Burst) {
		for i := range bs {
			got = appendAMBE(got, &bs[i])
		}
	}
	rx, err := NewReceiver(&RxConfig{Tier: tier, Rate: recordingRate, Format: IQFormatCU8, OnBursts: on})
	if err != nil {
		return nil, err
	}
	var werr error
	err = eachChunk(path, 1<<16, func(b []byte) {
		if _, err := rx.Write(b); err != nil && werr == nil {
			werr = err
		}
	})
	if err == nil {
		err = werr
	}
	if err == nil {
		err = rx.Flush()
	}
	return got, err
}

func appendAMBE(got []uint64, r *Burst) []uint64 {
	data, _ := r.Decoded()
	var opts layer2.DecodeOptions
	if r.Voice != enums.VoiceBurstUnknown {
		opts.Kind, opts.VoiceBurst = layer2.KindVoice, r.Voice
	} else if r.Distance > layer2.DefaultSyncThreshold {
		opts.SyncThreshold = r.Distance
	}
	var b layer2.Burst
	if b.DecodeFromBytesWith(data, &opts) != nil || b.Kind != layer2.KindVoice {
		return got
	}
	for _, s := range b.VoiceSlots {
		f, _ := vocoder.DecodeAMBE72(s)
		got = append(got, rftx.AMBEValue(f))
	}
	return got
}

func eachLogLine(path string, f func([]string)) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		f(strings.Fields(sc.Text()))
	}
	return sc.Err()
}

func ipscTruth(path, dir string) ([]uint64, error) {
	var out []uint64
	err := eachLogLine(path, func(p []string) {
		if len(p) < 3 || p[1] != dir {
			return
		}
		b, err := hex.DecodeString(strings.TrimPrefix(p[len(p)-1], "body="))
		if err != nil || len(b) < 52 || b[0] != 0x80 && b[0] != 0x81 || b[30] != 0x0a && b[30] != 0x8a {
			return
		}
		for _, f := range vocoder.UnpackAMBEVoice([19]byte(b[33:52])) {
			out = append(out, rftx.AMBEValue(f))
		}
	})
	return out, err
}

func dmrdTruth(path string) ([]uint64, error) {
	var out []uint64
	err := eachLogLine(path, func(p []string) {
		if len(p) < 2 {
			return
		}
		b, err := hex.DecodeString(p[len(p)-1])
		if err != nil || len(b) < 53 || string(b[:4]) != "DMRD" || b[15]>>4&3 > 1 {
			return
		}
		fr, _ := layer2.RawBurst(b[20:53]).AMBEFrames()
		for _, f := range fr {
			out = append(out, rftx.AMBEValue(f))
		}
	})
	return out, err
}

func lcsLen(a, b []uint64) int {
	prev, cur := make([]int, len(b)+1), make([]int, len(b)+1)
	for _, x := range a {
		for j, y := range b {
			if x == y {
				cur[j+1] = prev[j] + 1
			} else {
				cur[j+1] = max(prev[j+1], cur[j])
			}
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func sliceTruth(call []uint64, outs [][]uint64, seconds float64) int {
	count := map[uint64]int{}
	at := map[uint64]int{}
	for i, v := range call {
		count[v]++
		at[v] = i
	}
	lo, hi := -1, -1
	for _, got := range outs {
		for _, v := range got {
			if count[v] != 1 {
				continue
			}
			if i := at[v]; lo < 0 || i < lo {
				lo = i
			}
			if i := at[v]; i > hi {
				hi = i
			}
		}
	}
	frames := int(math.Round(seconds/rftx.SlotSeconds/2)) * 3
	if lo < 0 {
		return 0
	}
	return max(hi-lo+1, min(frames, len(call)))
}

type cnSpectra struct {
	freqs []float64
	rows  [][]float32
}

type mixedFFT struct {
	n       int
	w       []complex128
	factors []int
	tmp     []complex128
}

func newMixedFFT(n int) *mixedFFT {
	f := &mixedFFT{n: n, w: make([]complex128, n), tmp: make([]complex128, n)}
	for k := range f.w {
		s, c := math.Sincos(-2 * math.Pi * float64(k) / float64(n))
		f.w[k] = complex(c, s)
	}
	for m := n; m > 1; {
		p := m
		for _, q := range []int{5, 4, 3, 2} {
			if m%q == 0 {
				p = q
				break
			}
		}
		f.factors = append(f.factors, p)
		m /= p
	}
	return f
}

func (f *mixedFFT) transform(out, in []complex128) {
	f.step(out, in, f.n, 1, f.factors)
}

func (f *mixedFFT) step(out, in []complex128, n, stride int, factors []int) {
	if n == 1 {
		out[0] = in[0]
		return
	}
	r, m := factors[0], n/factors[0]
	for j := range r {
		f.step(out[j*m:(j+1)*m], in[j*stride:], m, stride*r, factors[1:])
	}
	scale := f.n / n
	t := f.tmp[:r]
	for k := range m {
		for j := range r {
			t[j] = out[j*m+k]
		}
		for q := range r {
			var acc complex128
			for j := range r {
				acc += t[j] * f.w[(j*(k+q*m)*scale)%f.n]
			}
			out[q*m+k] = acc
		}
	}
}

func captureSpectra(path string, fs float64) (cnSpectra, error) {
	n := int(math.Round(fs * 0.005))
	var sp cnSpectra
	var keep []int
	for k := -n / 2; k < n-n/2; k++ {
		if hz := float64(k) * fs / float64(n); math.Abs(hz) <= 80e3 {
			sp.freqs = append(sp.freqs, hz)
			keep = append(keep, (k+n)%n)
		}
	}
	win := make([]float64, n)
	var w2 float64
	for i := range win {
		win[i] = float64(float32(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1))))
		w2 += win[i] * win[i]
	}
	norm := 1 / (fs * w2)
	fft := newMixedFFT(n)
	z, x := make([]complex128, 200*n), make([]complex128, n)
	err := eachChunk(path, 200*2*n, func(b []byte) {
		m := len(b) / (2 * n)
		z := z[:m*n]
		var mean complex128
		for i := range z {
			z[i] = complex((float64(b[2*i])-127.5)/128, (float64(b[2*i+1])-127.5)/128)
			mean += z[i]
		}
		mean /= complex(float64(len(z)), 0)
		for k := range m {
			blk := z[k*n : (k+1)*n]
			for i := range blk {
				blk[i] = (blk[i] - mean) * complex(win[i], 0)
			}
			fft.transform(x, blk)
			row := make([]float32, len(keep))
			for i, k := range keep {
				v := x[k]
				row[i] = float32((real(v)*real(v) + imag(v)*imag(v)) * norm)
			}
			sp.rows = append(sp.rows, row)
		}
	})
	return sp, err
}

func percentile(v []float64, p float64) float64 {
	s := slices.Clone(v)
	slices.Sort(s)
	x := p / 100 * float64(len(s)-1)
	i := int(x)
	if i+1 >= len(s) {
		return s[len(s)-1]
	}
	return s[i] + (x-float64(i))*(s[i+1]-s[i])
}

func erodeMask(m []bool) []bool {
	out := slices.Clone(m)
	for i := range m {
		if i > 0 && !m[i-1] || i+1 < len(m) && !m[i+1] {
			out[i] = false
		}
	}
	return out
}

func (sp cnSpectra) bandMean(row []float32, in func(int) bool) float64 {
	var s float64
	var k int
	for i, v := range row {
		if in(i) {
			s += float64(v)
			k++
		}
	}
	return s / float64(k)
}

func (sp cnSpectra) band(fc, a, b float64) func(int) bool {
	return func(i int) bool {
		d := math.Abs(sp.freqs[i] - fc)
		return d >= a && d <= b
	}
}

func (sp cnSpectra) meanOf(sel []bool) []float64 {
	m := make([]float64, len(sp.freqs))
	var k int
	for b, row := range sp.rows {
		if sel != nil && !sel[b] {
			continue
		}
		for i, v := range row {
			m[i] += float64(v)
		}
		k++
	}
	for i := range m {
		m[i] /= float64(k)
	}
	return m
}

func vecMean(v []float64, in func(int) bool) float64 {
	var s float64
	var k int
	for i, x := range v {
		if in(i) {
			s += x
			k++
		}
	}
	return s / float64(k)
}

func (sp cnSpectra) carrierToNoise() (float64, bool) {
	const block, chHalf, offMargin = 0.005, 6250.0, 2.5
	nb := len(sp.rows)
	if nb == 0 {
		return 0, false
	}
	df := sp.freqs[1] - sp.freqs[0]
	all := sp.meanOf(nil)
	floor := percentile(all, 50)
	w := int(math.RoundToEven(2 * chHalf / df))
	fc0 := sp.strongestChannel(all, floor, w)
	ch, flat := sp.band(fc0, 0, chHalf), sp.band(fc0, 40e3, 60e3)
	ref := func(i int) bool { return sp.freqs[i] >= fc0+40e3 && sp.freqs[i] < fc0+40e3+2*chHalf }
	pdb, fdb, rdb, pch := make([]float64, nb), make([]float64, nb), make([]float64, nb), make([]float64, nb)
	for b, row := range sp.rows {
		pch[b] = sp.bandMean(row, ch)
		pdb[b] = 10 * math.Log10(pch[b]+1e-30)
		fdb[b] = 10 * math.Log10(sp.bandMean(row, flat)+1e-30)
		rdb[b] = 10 * math.Log10(sp.bandMean(row, ref)+1e-30)
	}
	keyed := sp.keyedBlocks(pdb, rdb, pch, flat)
	on := erodeMask(keyed)
	notKeyed := make([]bool, nb)
	for i, k := range keyed {
		notKeyed[i] = !k
	}
	off := erodeMask(notKeyed)
	var nOn, nOff int
	for i := range off {
		off[i] = off[i] && pdb[i]-fdb[i] <= offMargin
		if off[i] {
			nOff++
		}
		if on[i] {
			nOn++
		}
	}
	if nOn == 0 {
		return 0, false
	}
	pOn := sp.meanOf(on)
	useOff := float64(nOff)*block >= 0.2
	var pOff []float64
	if useOff {
		pOff = sp.meanOf(off)
	}
	fc, n0, d := sp.refineCarrier(fc0, pOn, pOff, useOff, df, chHalf)
	var c float64
	chf := sp.band(fc, 0, chHalf)
	for i := range d {
		if chf(i) {
			c += d[i]
		}
	}
	c *= df
	if c <= 0 {
		return 0, false
	}
	return 10 * math.Log10(c/(n0*12500)), true
}

func (sp cnSpectra) strongestChannel(all []float64, floor float64, w int) float64 {
	fc0, best := 0.0, math.Inf(-1)
	for i := range all {
		if math.Abs(sp.freqs[i]) > 25e3 {
			continue
		}
		var c float64
		for m := i + (w-1)/2 - w + 1; m <= i+(w-1)/2; m++ {
			if m >= 0 && m < len(all) {
				c += max(all[m]-floor, 0)
			}
		}
		if c > best {
			fc0, best = sp.freqs[i], c
		}
	}
	return fc0
}

func (sp cnSpectra) keyedBlocks(pdb, rdb, pch []float64, flat func(int) bool) []bool {
	nb := len(pdb)
	keyed := make([]bool, nb)
	lo, hi := percentile(pdb, 5), percentile(pdb, 95)
	if hi-lo > max(3, percentile(rdb, 95)-percentile(rdb, 5)+1.5) {
		on, off := lo+0.6*(hi-lo), lo+0.4*(hi-lo)
		state := pdb[0] > (on+off)/2
		for i, v := range pdb {
			if state && v < off {
				state = false
			} else if !state && v > on {
				state = true
			}
			keyed[i] = state
		}
		for i := 0; i < nb; {
			if !keyed[i] {
				i++
				continue
			}
			j := i
			for j < nb && keyed[j] {
				j++
			}
			if j-i < 3 {
				clear(keyed[i:j])
			}
			i = j
		}
	} else {
		nflat := make([]float64, nb)
		for b, row := range sp.rows {
			nflat[b] = sp.bandMean(row, flat)
		}
		k := percentile(pch, 50)/percentile(nflat, 50) > 2
		for i := range keyed {
			keyed[i] = k
		}
	}
	return keyed
}

func (sp cnSpectra) refineCarrier(fc0 float64, pOn, pOff []float64, useOff bool, df, chHalf float64) (fc, n0 float64, d []float64) {
	fc = fc0
	d = make([]float64, len(pOn))
	for range 3 {
		ch := sp.band(fc, 0, chHalf)
		n0v := pOff
		if useOff {
			n0 = vecMean(pOff, ch)
		} else {
			fl := sp.band(fc, 40e3, 60e3)
			l := vecMean(pOn, func(i int) bool { return fl(i) && sp.freqs[i] < fc })
			r := vecMean(pOn, func(i int) bool { return fl(i) && sp.freqs[i] > fc })
			n0 = (l + r) / 2
			n0v = make([]float64, len(pOn))
			for i := range n0v {
				n0v[i] = n0
			}
		}
		var num, den float64
		for i := range pOn {
			d[i] = pOn[i] - n0v[i]
			if ch(i) {
				num += sp.freqs[i] * max(d[i], 0)
				den += max(d[i], 0)
			}
		}
		nfc := num / max(den, 1e-30)
		done := math.Abs(nfc-fc) < df/4
		fc = nfc
		if done {
			break
		}
	}
	return fc, n0, d
}

func recordingIQTiers() []int {
	var idx []int
	for i, e := range benchEntries() {
		if e.mode == benchIQ {
			idx = append(idx, i)
		}
	}
	return idx
}

// TestRxBenchRecordings scores the I/Q tiers on the real recordings and
// writes testdata/rxbench_recordings.json. DMRRF_RECORDINGS names the
// directory that holds the dmrgo-rfcap and ipsc2hbrp-rig captures; the test
// is skipped without it.
func TestRxBenchRecordings(t *testing.T) {
	t.Parallel()
	root := os.Getenv("DMRRF_RECORDINGS")
	if root == "" {
		t.Skip("DMRRF_RECORDINGS is not set")
	}
	if _, err := os.Stat(root); err != nil {
		t.Skip(err)
	}
	srcs := recordingSources()
	tiers := recordingIQTiers()
	entries := benchEntries()
	got := make([][][]uint64, len(srcs))
	cn := make([]float64, len(srcs))
	type job struct {
		rec, tier int
		cost      int64
	}
	jobs := make([]job, 0, (len(tiers)+1)*len(srcs))
	for i, s := range srcs {
		fi, err := os.Stat(filepath.Join(root, s.capture))
		if err != nil {
			t.Fatal(err)
		}
		size := fi.Size()
		if strings.HasSuffix(s.capture, ".zst") {
			size *= 3
		}
		got[i] = make([][]uint64, len(tiers))
		for k, ti := range tiers {
			jobs = append(jobs, job{i, k, size * []int64{1, 1, 4, 64}[entries[ti].number()]})
		}
		jobs = append(jobs, job{i, -1, size})
	}
	slices.SortStableFunc(jobs, func(a, b job) int { return int(b.cost>>20) - int(a.cost>>20) })
	queue := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				path := filepath.Join(root, srcs[j.rec].capture)
				if j.tier < 0 {
					sp, err := captureSpectra(path, recordingRate)
					v, ok := sp.carrierToNoise()
					mu.Lock()
					if err != nil || !ok {
						t.Errorf("%s: C/N: %v", srcs[j.rec].id, err)
					}
					cn[j.rec] = v
					mu.Unlock()
					continue
				}
				out, err := receiveRecording(path, entries[tiers[j.tier]].tier)
				mu.Lock()
				if err != nil {
					t.Errorf("%s tier %d: %v", srcs[j.rec].id, j.tier, err)
				}
				got[j.rec][j.tier] = out
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		queue <- j
	}
	close(queue)
	wg.Wait()
	if t.Failed() {
		return
	}
	rows := make([]recordingRow, len(srcs))
	for i, s := range srcs {
		var truth []uint64
		var err error
		if s.truth == truthIPSC {
			truth, err = ipscTruth(filepath.Join(root, s.log), s.dir)
		} else {
			truth, err = dmrdTruth(filepath.Join(root, s.log))
		}
		if err != nil {
			t.Fatal(err)
		}
		exact := slices.Repeat([]int{-1}, len(entries))
		best := 0
		for k, ti := range tiers {
			exact[ti] = lcsLen(truth, got[i][k])
			best = max(best, exact[ti])
		}
		n := len(truth)
		if s.sliceSeconds > 0 {
			n = max(sliceTruth(truth, got[i], s.sliceSeconds), best)
		}
		rows[i] = recordingRow{Name: s.name, CN: oneDecimal(math.Round(cn[i]*10) / 10), Truth: n, Exact: exact}
		t.Logf("%-16s C/N %5.1f dB  truth %4d  exact %v", s.id, cn[i], n, exact[tiers[0]:])
	}
	b, err := json.MarshalIndent(rows, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordingsPath, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
