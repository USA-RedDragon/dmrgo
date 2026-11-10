package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const m7Features = "+armv7e-m,+thumb-mode,+dsp,+hwdiv,+fpregs,+fp64,+fp16,+vfp2,+vfp2sp,+vfp3d16,+vfp3d16sp,+vfp4d16,+vfp4d16sp,+fp-armv8d16,+fp-armv8d16sp,-aes,-bf16,-cdecp0,-cdecp1,-cdecp2,-cdecp3,-cdecp4,-cdecp5,-cdecp6,-cdecp7,-crc,-crypto,-d32,-dotprod,-fp-armv8,-fp-armv8sp,-fp16fml,-fullfp16,-hwdiv-arm,-i8mm,-lob,-mve,-mve.fp,-neon,-pacbti,-ras,-sb,-sha2,-vfp3,-vfp3sp,-vfp4,-vfp4sp"

func (b *bench) setupM7(ld, startup string) error {
	path := filepath.Join(b.work, "m7.ld")
	if err := os.WriteFile(path, []byte(ld), 0o600); err != nil {
		return err
	}
	rel, err := filepath.Rel(b.t.tinygoRoot, path)
	if err != nil {
		return err
	}
	js, err := json.Marshal(map[string]any{
		"inherits":           []string{"cortex-m7"},
		"build-tags":         []string{"qemu", "mps2_an500"},
		"linkerscript":       rel,
		"default-stack-size": 32768,
		"extra-files":        []string{startup},
		"features":           m7Features,
		"cflags":             []string{"-mfloat-abi=softfp", "-mfpu=fpv5-d16"},
	})
	if err != nil {
		return err
	}
	b.m7 = filepath.Join(b.work, "m7.json")
	return os.WriteFile(b.m7, js, 0o600)
}

func (b *bench) bigTarget(target map[string]any, name, ld string) (string, error) {
	path := filepath.Join(b.work, name+".ld")
	if err := os.WriteFile(path, []byte(ld), 0o600); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(b.t.tinygoRoot, path)
	if err != nil {
		return "", err
	}
	target["linkerscript"] = rel
	js, err := json.Marshal(target)
	if err != nil {
		return "", err
	}
	out := filepath.Join(b.work, name+".json")
	return out, os.WriteFile(out, js, 0o600)
}

func (b *bench) rxTier(ctx context.Context, r benchRow) (measure, error) {
	return b.rxHeap(ctx, r, b.m7, "mps2-an500", "cortex-m7", "a", fmt.Sprintf("-t%d", r.tier))
}

func (b *bench) rxFull(ctx context.Context, r benchRow) (measure, error) {
	return b.rxHeap(ctx, r, b.m4full, "mps2-an386", "cortex-m4", "q", "")
}

func (b *bench) rxHeap(ctx context.Context, r benchRow, target, machine, cpu, prefix, suffix string) (measure, error) {
	on, err := b.buildFor(ctx, target, "leaking", r.prog, r.rate, r.tier, true)
	if err != nil {
		return measure{}, err
	}
	off, err := b.buildFor(ctx, target, "leaking", r.prog, r.rate, r.tier, false)
	if err != nil {
		return measure{}, err
	}
	img, err := readELF(on)
	if err != nil {
		return measure{}, err
	}
	offImg, err := readELF(off)
	if err != nil {
		return measure{}, err
	}
	var m measure
	var depth, heap uint64
	for _, c := range signalConds() {
		in := fmt.Sprintf("%s-%s-%d.bin", prefix, c, r.rate)
		name := in + suffix
		ref, ok := b.refs[name]
		if !ok {
			return m, fmt.Errorf("no reference %s", name)
		}
		dir := filepath.Join(b.work, "run", strings.TrimSuffix(in, ".bin")+suffix)
		o1, err := b.qemuOn(ctx, machine, cpu, on, dir, filepath.Join(b.sig, in))
		if err != nil {
			return m, err
		}
		if o1.bursts != ref.bursts || o1.hash != ref.hash {
			return m, fmt.Errorf("%s: the bench gave %s bursts with hash %s, the Go receiver %s with hash %s", name, o1.bursts, o1.hash, ref.bursts, ref.hash)
		}
		if o1.grow != 0 {
			return m, fmt.Errorf("%s: the receiver allocated %d B after it started", name, o1.grow)
		}
		o0, err := b.qemuOn(ctx, machine, cpu, off, dir, filepath.Join(b.sig, in))
		if err != nil {
			return m, err
		}
		c1, err := img.cycles(o1.counts, func(s string) bool { return s == "main.onBursts" || strings.HasPrefix(s, "main.mix") })
		if err != nil {
			return m, err
		}
		c0, err := offImg.cycles(o0.counts, func(string) bool { return false })
		if err != nil {
			return m, err
		}
		secs := float64(ref.samples) / float64(ref.rate)
		m.mhz = max(m.mhz, float64(c1.total-c1.matched-c0.total)/secs/1e6)
		depth, heap = max(depth, o1.depth), max(heap, o1.heap)
		_ = os.Remove(o1.counts)
		_ = os.Remove(o0.counts)
	}
	m.mhz = round1(m.mhz)
	m.ram = img.ram - img.mainData + depth + heap
	m.flashKB = round1(float64(img.flash) / 1000)
	return m, nil
}
