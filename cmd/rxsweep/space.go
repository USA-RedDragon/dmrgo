package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// variant is one receiver to measure: a base tier and what replaces its
// settings.
type variant struct {
	// Name labels the variant in the tables and graphs.
	Name string `json:"name"`
	// Tier is the base tier, Lookback Config.LookbackSlots and Hz
	// Config.BasebandHz (0 means 48000).
	Tier     int `json:"tier"`
	Lookback int `json:"lookback,omitempty"`
	Hz       int `json:"hz,omitempty"`
	// Disc receives the stream as FM discriminator audio with
	// rf.DiscReceiver, which runs the TierDisc settings.
	Disc bool `json:"disc,omitempty"`
	// Opt holds options fields of package rf that replace those of the
	// tier, by field name.
	Opt map[string]json.RawMessage `json:"opt,omitempty"`
	// Consts holds constants of package rf to set, by file and name, as Go
	// expressions.
	Consts map[string]map[string]string `json:"consts,omitempty"`
	// Edits holds source replacements of package rf, by file: each old text
	// must occur once.
	Edits map[string][][2]string `json:"edits,omitempty"`
}

// family generates variants from a base: Mode "each" changes one key at a
// time, Mode "product" takes every combination of the keys.
type family struct {
	Name string  `json:"name"`
	Base variant `json:"base"`
	Mode string  `json:"mode"`
	// Vary lists the values of each key: "tier", "lookback", "hz", "disc",
	// "opt.<Field>" or "consts.<file>.<Name>".
	Vary map[string][]json.RawMessage `json:"vary"`
}

// grid is a set of C/N points and noise seeds.
type grid struct {
	Lo    float64 `json:"lo"`
	Hi    float64 `json:"hi"`
	Step  float64 `json:"step"`
	Seeds int     `json:"seeds"`
}

// space is the content of a space file.
type space struct {
	// Superframes is the voice call length of the stream and Offset its
	// carrier offset in Hz.
	Superframes int     `json:"superframes"`
	Offset      float64 `json:"offset"`
	// Coarse is the first grid of every variant. Fine is a grid of
	// relative C/N, Lo to Hi around the 90% point of each of Payloads.
	Coarse   grid      `json:"coarse"`
	Fine     grid      `json:"fine"`
	Payloads []string  `json:"payloads"`
	Variants []variant `json:"variants"`
	Families []family  `json:"families"`
}

func loadSpace(path string) (*space, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	sp := &space{Superframes: 30, Offset: 1400,
		Coarse:   grid{Lo: -4, Hi: 14, Step: 1, Seeds: 2},
		Fine:     grid{Lo: -1, Hi: 1, Step: 0.25, Seeds: 6},
		Payloads: lossPayloads()}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(sp); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, f := range sp.Families {
		vs, err := f.expand()
		if err != nil {
			return nil, fmt.Errorf("family %s: %w", f.Name, err)
		}
		sp.Variants = append(sp.Variants, vs...)
	}
	seen := map[string]bool{}
	for _, v := range sp.Variants {
		if v.Name == "" || seen[v.Name] {
			return nil, fmt.Errorf("variant name %q is empty or not unique", v.Name)
		}
		seen[v.Name] = true
	}
	return sp, nil
}

// expand returns the variants of f.
func (f family) expand() ([]variant, error) {
	keys := make([]string, 0, len(f.Vary))
	for k := range f.Vary {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var out []variant
	switch f.Mode {
	case "each":
		out = append(out, f.Base)
		out[0].Name = f.Name + "-base"
		for _, k := range keys {
			for _, val := range f.Vary[k] {
				v := f.Base.clone()
				if err := v.set(k, val); err != nil {
					return nil, err
				}
				v.Name = f.Name + "-" + label(k, val)
				out = append(out, v)
			}
		}
	case "product":
		idx := make([]int, len(keys))
		for {
			v := f.Base.clone()
			parts := []string{f.Name}
			for i, k := range keys {
				val := f.Vary[k][idx[i]]
				if err := v.set(k, val); err != nil {
					return nil, err
				}
				parts = append(parts, label(k, val))
			}
			v.Name = strings.Join(parts, "-")
			out = append(out, v)
			i := 0
			for ; i < len(keys); i++ {
				idx[i]++
				if idx[i] < len(f.Vary[keys[i]]) {
					break
				}
				idx[i] = 0
			}
			if i == len(keys) {
				break
			}
		}
	default:
		return nil, fmt.Errorf("mode %q is not each or product", f.Mode)
	}
	return out, nil
}

// label returns the name part of key k at value val.
func label(k string, val json.RawMessage) string {
	k = k[strings.LastIndexByte(k, '.')+1:]
	return k + "=" + strings.Trim(string(val), `"`)
}

func (v variant) clone() variant {
	c := v
	c.Opt = maps.Clone(v.Opt)
	c.Consts = map[string]map[string]string{}
	for f, cs := range v.Consts {
		c.Consts[f] = maps.Clone(cs)
	}
	c.Edits = maps.Clone(v.Edits)
	return c
}

// set sets key k of v to val.
func (v *variant) set(k string, val json.RawMessage) error {
	switch {
	case k == "tier":
		return json.Unmarshal(val, &v.Tier)
	case k == "lookback":
		return json.Unmarshal(val, &v.Lookback)
	case k == "hz":
		return json.Unmarshal(val, &v.Hz)
	case k == "disc":
		return json.Unmarshal(val, &v.Disc)
	case strings.HasPrefix(k, "opt."):
		if v.Opt == nil {
			v.Opt = map[string]json.RawMessage{}
		}
		v.Opt[k[4:]] = val
		return nil
	case strings.HasPrefix(k, "consts."):
		rest := k[7:]
		i := strings.LastIndexByte(rest, '.')
		if i < 0 || !strings.HasSuffix(rest[:i], ".go") {
			return fmt.Errorf("key %q is not consts.<file>.go.<Name>", k)
		}
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			s = string(val)
		}
		if v.Consts == nil {
			v.Consts = map[string]map[string]string{}
		}
		if v.Consts[rest[:i]] == nil {
			v.Consts[rest[:i]] = map[string]string{}
		}
		v.Consts[rest[:i]][rest[i+1:]] = s
		return nil
	}
	return fmt.Errorf("unknown key %q", k)
}

// patched reports whether v changes the sources.
func (v *variant) patched() bool { return len(v.Consts) > 0 || len(v.Edits) > 0 }

// patchKey names the source tree of the patches of v.
func (v *variant) patchKey() string {
	h := sha256.New()
	for _, f := range slices.Sorted(maps.Keys(v.Consts)) {
		for _, name := range slices.Sorted(maps.Keys(v.Consts[f])) {
			fmt.Fprintf(h, "const %q %q %q\n", f, name, v.Consts[f][name])
		}
	}
	for _, f := range slices.Sorted(maps.Keys(v.Edits)) {
		for _, e := range v.Edits[f] {
			fmt.Fprintf(h, "edit %q %q %q\n", f, e[0], e[1])
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:6])
}

// apply writes the constants and edits of v into the package rf of the v2
// tree at dir.
func (v *variant) apply(dir string) error {
	files := map[string]bool{}
	for f := range v.Consts {
		files[f] = true
	}
	for f := range v.Edits {
		files[f] = true
	}
	for f := range files {
		path := filepath.Join(dir, "rf", f)
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(b)
		for name, expr := range v.Consts[f] {
			re := regexp.MustCompile(`(?m)^(\t` + regexp.QuoteMeta(name) + `(?:\s+\w+)?\s*=\s*)[^\n]+$`)
			if len(re.FindAllStringIndex(s, -1)) != 1 {
				return fmt.Errorf("%s: constant %s is not declared once as a line of a const block", f, name)
			}
			s = re.ReplaceAllString(s, "${1}"+strings.ReplaceAll(expr, "$", "$$"))
		}
		for _, e := range v.Edits[f] {
			if strings.Count(s, e[0]) != 1 {
				return fmt.Errorf("%s: edit text does not occur once: %q", f, e[0])
			}
			s = strings.Replace(s, e[0], e[1], 1)
		}
		if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// jobOpt returns the opt JSON of the worker job of v.
func (v *variant) jobOpt() json.RawMessage {
	if len(v.Opt) == 0 {
		return nil
	}
	b, err := json.Marshal(v.Opt)
	if err != nil {
		panic(err)
	}
	return b
}

// treeHash returns the hash of the Go sources and go.mod files of the v2
// tree at dir.
func treeHash(dir string) (string, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	fsys := root.FS()
	h := sha256.New()
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "testdata" {
			return fs.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") && d.Name() != "go.mod" && d.Name() != "go.sum" {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s %d\n", p, len(b))
		h.Write(b)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil)[:8]), err
}
