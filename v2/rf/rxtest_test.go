package rf

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func each(f func(*Burst)) func([]Burst) {
	if f == nil {
		return nil
	}
	return func(bs []Burst) {
		for i := range bs {
			f(&bs[i])
		}
	}
}

func mustNew(cfg RxConfig) *Receiver {
	r, err := NewReceiver(&cfg)
	if err != nil {
		panic(err)
	}
	return r
}

func mustNewOpt(cfg RxConfig, o options) *Receiver {
	r, err := newReceiver(cfg, &o)
	if err != nil {
		panic(err)
	}
	return r
}

func cf32(x []complex64) []byte {
	b := make([]byte, 8*len(x))
	for i, v := range x {
		binary.LittleEndian.PutUint32(b[8*i:], math.Float32bits(real(v)))
		binary.LittleEndian.PutUint32(b[8*i+4:], math.Float32bits(imag(v)))
	}
	return b
}

func (r *Receiver) collect(n int, run func()) []Burst {
	if r.mode == rxModeNone {
		return nil
	}
	if r.sink.on != nil {
		run()
		r.sink.deliver()
		return nil
	}
	r.sink.out = scratch(r, r.MaxOutput(n))
	run()
	out := r.sink.out
	r.sink.out = nil
	return out
}

var scratches sync.Map

func scratch(key any, n int) []Burst {
	if v, ok := scratches.Load(key); ok {
		if b, ok := v.(*[]Burst); ok && cap(*b) >= n {
			return (*b)[:0]
		}
	}
	b := make([]Burst, 0, n)
	scratches.Store(key, &b)
	return b
}

func (r *Receiver) writeBB(x []complex64) ([]Burst, error) {
	if r.mode == rxModeNone {
		return nil, ErrNotSetUp
	}
	return r.collect(len(x), func() {
		if r.mode == rxModeLow {
			r.low.writeBaseband(x)
			return
		}
		r.full.writeBaseband(x)
	}), nil
}

func (r *Receiver) writeIQ(x []byte) ([]Burst, error) {
	if r.mode == rxModeNone {
		return nil, ErrNotSetUp
	}
	return r.collect(len(x)/r.format.bytes()+1, func() { r.input(x) }), nil
}

func (r *Receiver) flushAll() []Burst {
	return r.collect(0, r.end)
}

func mustAudio(cfg AudioRxConfig) *AudioReceiver {
	r, err := NewAudioReceiver(&cfg)
	if err != nil {
		panic(err)
	}
	return r
}

func mustAudioOpt(cfg AudioRxConfig, o options) *AudioReceiver {
	r, err := newAudioReceiver(cfg, &o)
	if err != nil {
		panic(err)
	}
	return r
}

func (r *AudioReceiver) collect(n int, run func()) []Burst {
	if !r.set {
		return nil
	}
	if r.bursts.on != nil {
		run()
		r.bursts.deliver()
		return nil
	}
	r.bursts.out = scratch(r, r.MaxOutput(n))
	run()
	out := r.bursts.out
	r.bursts.out = nil
	return out
}

func (r *AudioReceiver) writeAll(x []int16) ([]Burst, error) {
	if !r.set {
		return nil, ErrNotSetUp
	}
	return r.collect(len(x), func() { r.feed(x) }), nil
}

func (r *AudioReceiver) flushAll() []Burst {
	return r.collect(0, r.flush)
}

func halfOpt(o *options) *options {
	h := *o
	h.IQHalf = true
	return &h
}

func iqFormatOf(signed bool) IQFormat {
	if signed {
		return IQFormatCS8
	}
	return IQFormatCU8
}

func ptr[T any](v T) *T { return &v }

func voiceFrames(bs []Burst) []uint64 {
	var out []uint64
	for i := range bs {
		var opts layer2.DecodeOptions
		if bs[i].Voice != 0 {
			opts.Kind, opts.VoiceBurst = layer2.KindVoice, bs[i].Voice
		}
		data, _ := bs[i].Decoded()
		var b layer2.Burst
		if err := b.DecodeFromBytesWith(data, &opts); err != nil || b.Kind != layer2.KindVoice {
			continue
		}
		frames, _ := data.AMBEFrames()
		for _, f := range frames {
			out = append(out, rftx.AMBEValue(f))
		}
	}
	return out
}

func c0c1InOrder(want, got []uint64) int {
	n, next := 0, 0
	for _, v := range got {
		for k := next; k < len(want); k++ {
			if want[k]>>25 == v>>25 {
				n, next = n+1, k+1
				break
			}
		}
	}
	return n
}

func (r *Receiver) host() *rxHost {
	if h, ok := r.full.(*rxHost); ok {
		return h
	}
	return &rxHost{}
}
