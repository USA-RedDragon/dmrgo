package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"
)

type encoding int

const (
	encU8 encoding = iota
	encS8
	encS16
	encF32
)

func (e encoding) size() int {
	switch e {
	case encU8, encS8:
		return 1
	case encS16:
		return 2
	case encF32:
		return 4
	}
	return 1
}

const (
	nameCU8    = "cu8"
	nameCS8    = "cs8"
	nameCS16   = "cs16le"
	nameCF32   = "cf32le"
	nameS16    = "s16le"
	nameWAV    = "wav"
	formatList = "cu8, cs8, cs16le, cf32le, s16le or wav"
)

type sampleFormat struct {
	name     string
	enc      encoding
	channels int
	wav      bool
}

func (f sampleFormat) iq() bool { return f.channels == 2 }

func (f sampleFormat) frame() int { return f.channels * f.enc.size() }

const formatUsage = "sample format: " + formatList + "; cu8 to cf32le are I/Q, s16le and wav FM audio (default from a WAV header or the file extension)"

func formatByName(name string) (sampleFormat, error) {
	switch name {
	case nameCU8:
		return sampleFormat{name: name, enc: encU8, channels: 2}, nil
	case nameCS8:
		return sampleFormat{name: name, enc: encS8, channels: 2}, nil
	case nameCS16, "cs16":
		return sampleFormat{name: nameCS16, enc: encS16, channels: 2}, nil
	case nameCF32, "cf32":
		return sampleFormat{name: nameCF32, enc: encF32, channels: 2}, nil
	case nameS16:
		return sampleFormat{name: name, enc: encS16, channels: 1}, nil
	case nameWAV:
		return sampleFormat{name: name, enc: encS16, channels: 1, wav: true}, nil
	}
	return sampleFormat{}, fmt.Errorf("-format %q, want %s", name, formatList)
}

func formatOfExtension(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cu8":
		return nameCU8
	case ".cs8":
		return nameCS8
	case ".cs16":
		return nameCS16
	case ".cf32", ".fc32", ".cfile":
		return nameCF32
	case ".wav":
		return nameWAV
	}
	return ""
}

func formatFor(name, path string) (sampleFormat, error) {
	if name != "" {
		return formatByName(name)
	}
	if n := formatOfExtension(path); n != "" {
		return formatByName(n)
	}
	return sampleFormat{}, fmt.Errorf("cannot tell the sample format of %q from its name; give -format", path)
}

func normalize(e encoding, b []byte) float64 {
	switch e {
	case encU8:
		return (float64(b[0]) - 127.5) / 127.5
	case encS8:
		v := int(b[0])
		return float64(v-v>>7<<8) / 127
	case encS16:
		v := int(binary.LittleEndian.Uint16(b))
		return float64(v-v>>15<<16) / 32767
	case encF32:
	}
	return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
}

func quantize(v, full float64) int {
	return int(math.Round(math.Max(-full, math.Min(full, v*full))))
}

func appendSample(dst []byte, e encoding, v float64) []byte {
	switch e {
	case encU8:
		return append(dst, byte(int(math.Round(math.Max(0, math.Min(255, v*127.5+127.5))))&0xFF))
	case encS8:
		return append(dst, byte(quantize(v, 127)&0xFF))
	case encS16:
		return binary.LittleEndian.AppendUint16(dst, uint16(quantize(v, 32767)&0xFFFF))
	case encF32:
	}
	return binary.LittleEndian.AppendUint32(dst, math.Float32bits(float32(v)))
}

const wavHeaderSize = 44

func wavHeader(rate float64, channels, samples int) ([]byte, error) {
	if rate != math.Round(rate) || rate < 1 || rate > math.MaxUint32 {
		return nil, fmt.Errorf("a WAV file needs a whole sample rate, not %g", rate)
	}
	data := samples * channels * 2
	if data > math.MaxUint32-wavHeaderSize {
		return nil, errors.New("too long for a WAV file")
	}
	b := make([]byte, 0, wavHeaderSize)
	b = append(b, "RIFF"...)
	b = binary.LittleEndian.AppendUint32(b, uint32((wavHeaderSize-8+data)&0xFFFFFFFF))
	b = append(b, "WAVEfmt "...)
	b = binary.LittleEndian.AppendUint32(b, 16)
	b = binary.LittleEndian.AppendUint16(b, 1)
	b = binary.LittleEndian.AppendUint16(b, uint16(channels&0xFFFF))
	b = binary.LittleEndian.AppendUint32(b, uint32(rate))
	b = binary.LittleEndian.AppendUint32(b, uint32(rate)*uint32(channels&0xFFFF)*2)
	b = binary.LittleEndian.AppendUint16(b, uint16((channels*2)&0xFFFF))
	b = binary.LittleEndian.AppendUint16(b, 16)
	b = append(b, "data"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(data&0xFFFFFFFF))
	return b, nil
}

func isWAV(r *bufio.Reader) bool {
	b, err := r.Peek(12)
	return err == nil && string(b[:4]) == "RIFF" && string(b[8:]) == "WAVE"
}

func readWAVHeader(r *bufio.Reader) (sampleFormat, float64, io.Reader, error) {
	var hdr [12]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return sampleFormat{}, 0, nil, fmt.Errorf("WAV header: %w", err)
	}
	if string(hdr[:4]) != "RIFF" || string(hdr[8:]) != "WAVE" {
		return sampleFormat{}, 0, nil, errors.New("not a WAV file")
	}
	var f sampleFormat
	var rate float64
	haveFmt := false
	for {
		var ch [8]byte
		if _, err := io.ReadFull(r, ch[:]); err != nil {
			return sampleFormat{}, 0, nil, fmt.Errorf("WAV header: %w", err)
		}
		size := binary.LittleEndian.Uint32(ch[4:])
		switch string(ch[:4]) {
		case "fmt ":
			if size < 16 || size > 1<<16 {
				return sampleFormat{}, 0, nil, fmt.Errorf("WAV fmt chunk of %d bytes", size)
			}
			b := make([]byte, size+size&1)
			if _, err := io.ReadFull(r, b); err != nil {
				return sampleFormat{}, 0, nil, fmt.Errorf("WAV header: %w", err)
			}
			tag := binary.LittleEndian.Uint16(b)
			if tag == 0xFFFE && size >= 26 {
				tag = binary.LittleEndian.Uint16(b[24:])
			}
			f.channels = int(binary.LittleEndian.Uint16(b[2:]))
			rate = float64(binary.LittleEndian.Uint32(b[4:]))
			bits := binary.LittleEndian.Uint16(b[14:])
			switch {
			case tag == 1 && bits == 8:
				f.enc = encU8
			case tag == 1 && bits == 16:
				f.enc = encS16
			case tag == 3 && bits == 32:
				f.enc = encF32
			default:
				return sampleFormat{}, 0, nil, fmt.Errorf("WAV format %d with %d bits, want 8-bit or 16-bit PCM or 32-bit float", tag, bits)
			}
			if f.channels != 1 && f.channels != 2 {
				return sampleFormat{}, 0, nil, fmt.Errorf("WAV with %d channels, want 1 (audio) or 2 (I/Q)", f.channels)
			}
			haveFmt = true
		case "data":
			if !haveFmt {
				return sampleFormat{}, 0, nil, errors.New("WAV data before its fmt chunk")
			}
			f.name, f.wav = nameWAV, true
			var body io.Reader = r
			if size != 0 && size != math.MaxUint32 {
				body = io.LimitReader(r, int64(size))
			}
			return f, rate, body, nil
		default:
			if _, err := io.CopyN(io.Discard, r, int64(size)+int64(size&1)); err != nil {
				return sampleFormat{}, 0, nil, fmt.Errorf("WAV header: %w", err)
			}
		}
	}
}

func txHint(f sampleFormat, path, freq string, rate float64) string {
	switch f.name {
	case nameCS8:
		return fmt.Sprintf("hackrf_transfer -t %s -f %s -s %.0f -x 0", path, freq, rate)
	case nameCS16:
		return fmt.Sprintf("tx_sdr -F CS16 -f %s -s %.0f %s", freq, rate, path)
	}
	return ""
}
