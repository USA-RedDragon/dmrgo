package main

import (
	"bytes"
	"strings"

	"github.com/klauspost/compress/zstd"
)

type encoding uint8

const (
	encScreen encoding = iota
	encScreenZstd
	encFields
	encFieldsZstd
	encFieldsDict
	numEncodings
)

func (e encoding) String() string {
	switch e {
	case encScreen:
		return "screen"
	case encScreenZstd:
		return "screen+zstd"
	case encFields:
		return "fields"
	case encFieldsZstd:
		return "fields+zstd"
	case encFieldsDict:
		return "fields+zstd+dict"
	case numEncodings:
	}
	return "?"
}

var zstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd} //nolint:gochecknoglobals // constant bytes

type codec struct {
	plain, dict       *zstd.Encoder
	plainDec, dictDec *zstd.Decoder
	scratch           []byte
}

func newCodec(dictID uint32, dict []byte) (*codec, error) {
	opts := []zstd.EOption{
		zstd.WithEncoderLevel(zstd.SpeedBestCompression),
		zstd.WithEncoderCRC(false),
		zstd.WithSingleSegment(true),
		zstd.WithEncoderConcurrency(1),
	}
	plain, err := zstd.NewWriter(nil, opts...)
	if err != nil {
		return nil, err
	}
	withDict, err := zstd.NewWriter(nil, append(opts, zstd.WithEncoderDictRaw(dictID, dict))...)
	if err != nil {
		return nil, err
	}
	plainDec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return nil, err
	}
	dictDec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderDictRaw(dictID, dict))
	if err != nil {
		return nil, err
	}
	return &codec{plain: plain, dict: withDict, plainDec: plainDec, dictDec: dictDec}, nil
}

func (c *codec) close() {
	c.plainDec.Close()
	c.dictDec.Close()
}

func compressed(dst []byte, enc *zstd.Encoder, src []byte) []byte {
	n := len(dst)
	dst = enc.EncodeAll(src, dst)
	return append(dst[:n], dst[n+len(zstdMagic):]...)
}

func (c *codec) encode(dst []byte, e encoding, f *form) []byte {
	switch e {
	case encScreen:
		return append(dst, f.screen()...)
	case encScreenZstd:
		return compressed(dst, c.plain, f.screen())
	case encFields:
		return f.appendFields(dst)
	case encFieldsZstd:
		c.scratch = f.appendFields(c.scratch[:0])
		return compressed(dst, c.plain, c.scratch)
	case encFieldsDict:
		c.scratch = f.appendFields(c.scratch[:0])
		return compressed(dst, c.dict, c.scratch)
	case numEncodings:
	}
	return dst
}

func (c *codec) decompress(dec *zstd.Decoder, body []byte) ([]byte, error) {
	c.scratch = append(append(c.scratch[:0], zstdMagic...), body...)
	return dec.DecodeAll(c.scratch, nil)
}

func (c *codec) decode(e encoding, t *template, body []byte) (form, error) {
	switch e {
	case encScreen:
		return parseScreen(t, body)
	case encScreenZstd:
		b, err := c.decompress(c.plainDec, body)
		if err != nil {
			return form{}, err
		}
		return parseScreen(t, b)
	case encFields:
		return parseFields(t, body)
	case encFieldsZstd, encFieldsDict:
		dec := c.plainDec
		if e == encFieldsDict {
			dec = c.dictDec
		}
		b, err := c.decompress(dec, body)
		if err != nil {
			return form{}, err
		}
		return parseFields(t, b)
	case numEncodings:
	}
	return form{}, errForm
}

func parseScreen(t *template, b []byte) (form, error) {
	rows := bytes.Split(b, []byte{'\n'})
	f := form{tmpl: t, values: make([]string, len(t.fields))}
	for i, fd := range t.fields {
		if fd.row >= len(rows) {
			return form{}, errForm
		}
		row := rows[fd.row]
		at := fd.col + len(fd.label) + 2
		if at > len(row) {
			continue
		}
		end := min(at+fd.width, len(row))
		f.values[i] = strings.TrimRight(string(row[at:end]), " ")
	}
	return f, nil
}

func trainDictionary(t *template, seed uint64) []byte {
	rng := newRand(seed)
	var d []byte
	for len(d) < dictSize {
		f := randomForm(rng, t)
		d = f.appendFields(d)
	}
	return d[:dictSize]
}
