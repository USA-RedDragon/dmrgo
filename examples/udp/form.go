package main

import (
	"errors"
	"math/rand/v2"
	"strings"
)

type field struct {
	label    string
	row, col int
	width    int
}

type template struct {
	id, version byte
	title       string
	fields      []field
}

func serviceOrder() template {
	return template{
		id: 1, version: 1,
		title: "SERVICE ORDER                                         EMCOMM DISPATCH",
		fields: []field{
			{"ORDER NO", 3, 2, 8},
			{"UNIT", 3, 30, 6},
			{"PRIORITY", 3, 50, 1},
			{"STATUS", 5, 2, 12},
			{"TIME", 5, 30, 5},
			{"STREET", 7, 2, 40},
			{"CITY", 9, 2, 20},
			{"GRID", 9, 30, 6},
			{"CONTACT", 11, 2, 24},
			{"PHONE", 11, 40, 12},
			{"NEED", 13, 2, 20},
			{"QTY", 13, 30, 4},
			{"NOTES", 15, 2, 71},
		},
	}
}

type form struct {
	tmpl   *template
	values []string
}

var errForm = errors.New("bad form")

const (
	maxField   = 255
	screenRows = 24
	screenCols = 80
)

func pick(rng *rand.Rand, s []string) string {
	return s[rng.IntN(len(s))]
}

func randomForm(rng *rand.Rand, t *template) form {
	streets := []string{"MAIN ST", "OAK AVE", "ELM ST", "HWY 59", "FM 1960", "CR 412", "MARKET ST", "RIVER RD", "PINE LN", "CEDAR DR"}
	cities := []string{"TOMBALL", "CONROE", "HUMBLE", "KATY", "SPRING", "CYPRESS", "MAGNOLIA", "WILLIS"}
	subsquares := strings.Split("ABCDEFGHIJKLMNOPQRSTUVWX", "")
	statuses := []string{"OPEN", "ENROUTE", "ON SCENE", "COMPLETE", "NEED ASSIST", "CANCELLED"}
	needs := []string{"WATER", "GENERATOR", "SANDBAGS", "MEDICAL", "SHELTER", "FUEL", "TARPS", "TRANSPORT"}
	first := []string{"J SMITH", "M GARCIA", "L NGUYEN", "R JOHNSON", "A PATEL", "K BROWN", "D LEE"}
	notes := []string{
		"ROAD FLOODED AT CROSSING USE ALT ROUTE",
		"ELDERLY RESIDENT NEEDS OXYGEN CHECK",
		"POWER OUT SINCE 0400 TREE ON LINE",
		"GATE CODE 4471 DOG IN YARD",
		"",
		"CALL ON ARRIVAL",
		"SHELTER AT CAPACITY REDIRECT TO HIGH SCHOOL",
		"WATER OVER ROAD 18 IN DO NOT CROSS",
	}
	v := make([]string, len(t.fields))
	for i, f := range t.fields {
		switch f.label {
		case "ORDER NO":
			v[i] = "SO" + digits(rng, 6)
		case "UNIT":
			v[i] = "MU" + digits(rng, 2)
		case "PRIORITY":
			v[i] = digits(rng, 1)
		case "STATUS":
			v[i] = pick(rng, statuses)
		case "TIME":
			v[i] = digits(rng, 2) + ":" + digits(rng, 2)
		case "STREET":
			v[i] = digits(rng, 1+rng.IntN(5)) + " " + pick(rng, streets)
		case "CITY":
			v[i] = pick(rng, cities)
		case "GRID":
			v[i] = "EL29" + pick(rng, subsquares) + pick(rng, subsquares)
		case "CONTACT":
			v[i] = pick(rng, first)
		case "PHONE":
			v[i] = "281" + digits(rng, 7)
		case "NEED":
			v[i] = pick(rng, needs)
		case "QTY":
			v[i] = digits(rng, 1+rng.IntN(2))
		case "NOTES":
			v[i] = pick(rng, notes)
		}
		if len(v[i]) > f.width {
			v[i] = v[i][:f.width]
		}
	}
	return form{tmpl: t, values: v}
}

func digits(rng *rand.Rand, n int) string {
	const set = "0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = set[rng.IntN(len(set))]
	}
	return string(b)
}

func (f *form) screen() []byte {
	var rows [screenRows][screenCols]byte
	for r := range rows {
		for c := range rows[r] {
			rows[r][c] = ' '
		}
	}
	copy(rows[0][:], f.tmpl.title)
	for i, fd := range f.tmpl.fields {
		s := fd.label + ": " + f.values[i]
		copy(rows[fd.row][fd.col:], s)
	}
	var b strings.Builder
	for r := range rows {
		b.WriteString(strings.TrimRight(string(rows[r][:]), " "))
		b.WriteByte('\n')
	}
	return []byte(strings.TrimRight(b.String(), "\n"))
}

func (f *form) appendFields(dst []byte) []byte {
	for _, v := range f.values {
		n := min(len(v), maxField)
		dst = append(dst, byte(n))
		dst = append(dst, v[:n]...)
	}
	return dst
}

func parseFields(t *template, b []byte) (form, error) {
	f := form{tmpl: t, values: make([]string, len(t.fields))}
	for i := range f.values {
		if len(b) == 0 || int(b[0]) > len(b)-1 {
			return form{}, errForm
		}
		n := int(b[0])
		f.values[i] = string(b[1 : 1+n])
		b = b[1+n:]
	}
	if len(b) != 0 {
		return form{}, errForm
	}
	return f, nil
}
