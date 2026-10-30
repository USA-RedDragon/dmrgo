package main

import (
	"fmt"
	"regexp"
	"strings"
)

type marker struct {
	kind   string
	args   map[string]string
	raw    string
	line   int
	start  int
	end    int
	body   string
	inline bool
}

var openTag = regexp.MustCompile(`<!-- dmrgo:([a-z]+)((?:\s+[a-z]+=(?:"[^"]*"|[^\s>"]+))*)\s*-->`)

var argRe = regexp.MustCompile(`([a-z]+)=(?:"([^"]*)"|([^\s>"]+))`)

const (
	kindOutput = "output"
	kindCode   = "code"
	kindStates = "states"
	kindConfig = "config"
	kindConst  = "const"
	kindErrors = "errors"
	kindDoc    = "doc"
	kindGoMod  = "gomod"
	kindFlags  = "flags"
	argPkg     = "pkg"
)

func kindArgs(kind string) ([]string, bool) {
	switch kind {
	case kindOutput:
		return []string{"example", "test"}, true
	case kindCode:
		return []string{"file"}, true
	case kindStates, kindConfig, kindErrors:
		return []string{argPkg, "type"}, true
	case kindConst:
		return []string{argPkg, "name"}, true
	case kindDoc:
		return []string{argPkg}, true
	case kindGoMod:
		return []string{"field"}, true
	case kindFlags:
		return []string{"cmd"}, true
	}
	return nil, false
}

// maskFences returns text with the lines of fenced code blocks outside
// markers replaced by spaces, so that a marker shown as an example in a
// code block is not a marker.
func maskFences(text string) string {
	b := []byte(text)
	fence := ""
	inMarker := false
	start := 0
	for start < len(b) {
		end := start
		for end < len(b) && b[end] != '\n' {
			end++
		}
		line := strings.TrimSpace(string(b[start:end]))
		switch {
		case fence == "" && !inMarker && openTag.MatchString(line):
			inMarker = !strings.Contains(line, "<!-- /dmrgo:")
		case fence == "" && inMarker && strings.Contains(line, "<!-- /dmrgo:"):
			inMarker = false
		case fence == "" && !inMarker && (strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")):
			fence = line[:3]
		case fence != "" && strings.HasPrefix(line, fence):
			fence = ""
			for i := start; i < end; i++ {
				b[i] = ' '
			}
		}
		if fence != "" {
			for i := start; i < end; i++ {
				b[i] = ' '
			}
		}
		start = end + 1
	}
	return string(b)
}

func parseMarkers(text string) ([]marker, error) {
	var out []marker
	pos := 0
	masked := maskFences(text)
	for {
		loc := openTag.FindStringSubmatchIndex(masked[pos:])
		if loc == nil {
			return out, nil
		}
		m := marker{
			kind: text[pos+loc[2] : pos+loc[3]],
			raw:  strings.TrimSpace(text[pos+loc[4] : pos+loc[5]]),
			args: map[string]string{},
			line: strings.Count(text[:pos+loc[0]], "\n") + 1,
		}
		want, ok := kindArgs(m.kind)
		if !ok {
			return nil, fmt.Errorf("line %d: unknown marker kind dmrgo:%s", m.line, m.kind)
		}
		for _, kv := range argRe.FindAllStringSubmatch(m.raw, -1) {
			m.args[kv[1]] = kv[2] + kv[3]
		}
		if err := checkArgs(m, want); err != nil {
			return nil, err
		}
		m.start = pos + loc[1]
		closeTag := "<!-- /dmrgo:" + m.kind + " -->"
		rel := strings.Index(masked[m.start:], closeTag)
		if rel < 0 {
			return nil, fmt.Errorf("line %d: dmrgo:%s has no %s", m.line, m.kind, closeTag)
		}
		m.end = m.start + rel
		m.body = text[m.start:m.end]
		if next := openTag.FindStringIndex(m.body); next != nil {
			return nil, fmt.Errorf("line %d: dmrgo:%s holds another marker before its %s", m.line, m.kind, closeTag)
		}
		m.inline = m.kind == kindConst || m.kind == kindGoMod
		if !m.inline && !strings.HasPrefix(m.body, "\n") && m.body != "" {
			return nil, fmt.Errorf("line %d: dmrgo:%s needs its body on the lines after the open tag", m.line, m.kind)
		}
		out = append(out, m)
		pos = m.end + len(closeTag)
	}
}

func checkArgs(m marker, want []string) error {
	if m.kind == kindOutput {
		_, ex := m.args["example"]
		_, te := m.args["test"]
		if ex == te || len(m.args) != 1 {
			return fmt.Errorf("line %d: dmrgo:output takes exactly one of example= or test=", m.line)
		}
		return nil
	}
	allowed := map[string]bool{}
	for _, k := range want {
		allowed[k] = true
		if m.args[k] == "" {
			return fmt.Errorf("line %d: dmrgo:%s needs %s=", m.line, m.kind, k)
		}
	}
	if m.kind == kindStates {
		allowed["run"] = true
	}
	if m.kind == kindCode {
		n := 0
		for _, k := range []string{"lines", "symbol", "region"} {
			if _, ok := m.args[k]; ok {
				n++
			}
		}
		if n > 1 {
			return fmt.Errorf("line %d: dmrgo:code takes one of lines=, symbol= and region=", m.line)
		}
		allowed["lines"], allowed["symbol"], allowed["region"] = true, true, true
	}
	if m.kind == kindDoc {
		allowed["name"], allowed["section"] = true, true
	}
	if m.kind == kindGoMod {
		allowed["file"] = true
	}
	for k := range m.args {
		if !allowed[k] {
			return fmt.Errorf("line %d: dmrgo:%s does not take %s=", m.line, m.kind, k)
		}
	}
	return nil
}

func (m marker) String() string {
	return "dmrgo:" + m.kind + " " + m.raw
}

func wrapBody(m marker, content string) string {
	if m.inline {
		return content
	}
	return "\n" + strings.TrimRight(content, "\n") + "\n"
}
