package capigen

import (
	"os"
	"regexp"
	"strings"
)

func handABI(path string) ([]string, []string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	src := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(string(b), " ")
	var body, defs []string
	skip := 0
	for _, l := range strings.Split(src, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "#ifdef __cplusplus"):
			skip++
		case skip > 0 && strings.HasPrefix(t, "#endif"):
			skip--
		case skip > 0:
		case strings.HasPrefix(t, "#define "):
			f := strings.Fields(t)
			if len(f) > 2 {
				defs = append(defs, "sym hand "+strings.Join(f, " "))
			}
		case strings.HasPrefix(t, "#"):
		default:
			body = append(body, t)
		}
	}
	text := strings.Join(strings.Fields(strings.Join(body, " ")), " ")
	var syms, funcs []string
	depth, start := 0, 0
	proto := regexp.MustCompile(`\b(dmrgo_\w+)\s*\(`)
	for i, c := range text {
		switch c {
		case '{':
			depth++
		case '}':
			depth--
		case ';':
			if depth != 0 {
				continue
			}
			st := strings.TrimSpace(text[start : i+1])
			start = i + 1
			syms = append(syms, "sym hand "+st)
			if !strings.HasPrefix(st, "typedef") && !strings.HasPrefix(st, "enum") && !strings.HasPrefix(st, "struct") {
				if m := proto.FindStringSubmatch(st); m != nil {
					funcs = append(funcs, m[1])
				}
			}
		}
	}
	return append(syms, defs...), funcs, nil
}
