// Command inspector is a packet inspector for the browser: it decodes the
// network protocols that dmrgo supports, and the DMR bursts and data inside
// them, into a field tree with the bit ranges of each field. Built for
// GOOS=js GOARCH=wasm it serves index.html; built for the host it prints the
// decode of a file as JSON.
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// Range is the bits Start to End, End not included, of buffer Buf of a
// packet.
type Range struct {
	Buf   int `json:"b"`
	Start int `json:"s"`
	End   int `json:"e"`
}

// Node is one field of the tree: its name, its value as text, the bits it
// takes, an error with its reason, and its subfields.
type Node struct {
	Name     string  `json:"n"`
	Value    string  `json:"v,omitempty"`
	Ranges   []Range `json:"r,omitempty"`
	Err      string  `json:"err,omitempty"`
	Children []*Node `json:"c,omitempty"`
}

// Buffer is the octets that a decode step shows: the packet, or a payload
// that a step derived from it, such as the BPTC payload of a burst. Map,
// when set, gives the octet of buffer Parent that each octet came from.
type Buffer struct {
	Name   string `json:"name"`
	Hex    string `json:"hex"`
	Parent int    `json:"parent"`
	Map    []int  `json:"map,omitempty"`
}

// Packet is the decode of one datagram or frame.
type Packet struct {
	Index    int      `json:"index"`
	Meta     string   `json:"meta,omitempty"`
	Protocol string   `json:"protocol"`
	Summary  string   `json:"summary"`
	Buffers  []Buffer `json:"buffers"`
	Tree     []*Node  `json:"tree"`
	Errors   []string `json:"errors,omitempty"`
}

// Result is the decode of an input.
type Result struct {
	Input   string   `json:"input"`
	Packets []Packet `json:"packets"`
	Errors  []string `json:"errors,omitempty"`
}

// Options select the input form and the protocol. The zero value detects
// both.
type Options struct {
	// Format is "", "auto", "hex", "pcap" or "binary".
	Format string
	// Protocol is "", "auto", "ipsc", "dmrd", "hytera", "mmdvm", "hdap" or
	// "burst".
	Protocol string
}

// Inspect decodes input, a text of hex packets, a pcap or pcapng file, or
// a binary dump, and returns the decode of each packet.
func Inspect(input []byte, opt Options) Result {
	dgs, form, errs := readInput(input, opt.Format)
	res := Result{Input: form, Errors: errs}
	s := newSession(opt.Protocol)
	for i, d := range dgs {
		p := s.decode(d)
		p.Index = i
		p.Tree = trim(p.Tree, p.Buffers)
		res.Packets = append(res.Packets, p)
	}
	return res
}

// InspectJSON is Inspect with its result as JSON.
func InspectJSON(input []byte, opt Options) string {
	b, err := json.Marshal(Inspect(input, opt))
	if err != nil {
		return fmt.Sprintf(`{"errors":[%q]}`, err.Error())
	}
	return string(b)
}

type packetBuilder struct {
	p *Packet
}

func newPacket(proto string, data []byte, meta string) *packetBuilder {
	b := &packetBuilder{p: &Packet{Protocol: proto, Meta: meta}}
	b.buffer("packet", data, -1, nil)
	return b
}

func (b *packetBuilder) buffer(name string, data []byte, parent int, m []int) int {
	b.p.Buffers = append(b.p.Buffers, Buffer{Name: name, Hex: hex.EncodeToString(data), Parent: parent, Map: m})
	return len(b.p.Buffers) - 1
}

func (b *packetBuilder) errorf(format string, args ...any) {
	b.p.Errors = append(b.p.Errors, fmt.Sprintf(format, args...))
}

func (b *packetBuilder) summary(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	if b.p.Summary == "" {
		b.p.Summary = s
		return
	}
	if strings.Contains(b.p.Summary, s) {
		return
	}
	b.p.Summary += "; " + s
}

func field(name, value string, start, end int) *Node {
	n := &Node{Name: name, Value: value}
	if end > start {
		n.Ranges = []Range{{Buf: 0, Start: start * 8, End: end * 8}}
	}
	return n
}

func group(name, value string, children ...*Node) *Node {
	return &Node{Name: name, Value: value, Children: children}
}

func findNode(ns []*Node, path string) *Node {
	name, rest, more := strings.Cut(path, ".")
	for _, n := range ns {
		if n.Name == name {
			if !more {
				return n
			}
			return findNode(n.Children, rest)
		}
	}
	return nil
}

func trim(ns []*Node, bufs []Buffer) []*Node {
	out := ns[:0]
	for _, n := range ns {
		fits := true
		for _, r := range n.Ranges {
			if r.Buf < 0 || r.Buf >= len(bufs) || r.End > len(bufs[r.Buf].Hex)*4 {
				fits = false
			}
		}
		if !fits && n.Err == "" {
			continue
		}
		n.Children = trim(n.Children, bufs)
		out = append(out, n)
	}
	return out
}
