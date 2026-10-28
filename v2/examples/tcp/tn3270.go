package main

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

const (
	iac      = 255
	tnDo     = 253
	tnWill   = 251
	tnSB     = 250
	tnSE     = 240
	tnEOR    = 239
	optBin   = 0
	optTType = 24
	optEOR   = 25

	cmdEraseWrite = 0xF5
	orderSBA      = 0x11
	orderSF       = 0x1D
	orderIC       = 0x13
	aidEnter      = 0x7D
	aidPF3        = 0xF3
	wcc           = 0xC3
	attrProtected = 0x60
	attrInput     = 0x40
	cols          = 80
)

type field struct {
	label    string
	row, col int
	width    int
}

func formFields() []field {
	return []field{
		{"ORDER NO", 3, 2, 8}, {"UNIT", 3, 30, 6}, {"PRIORITY", 3, 50, 1},
		{"STATUS", 5, 2, 12}, {"TIME", 5, 30, 5}, {"STREET", 7, 2, 40},
		{"CITY", 9, 2, 20}, {"GRID", 9, 30, 6}, {"CONTACT", 11, 2, 24},
		{"PHONE", 11, 40, 12}, {"NEED", 13, 2, 20}, {"QTY", 13, 30, 4},
		{"NOTES", 15, 2, 71},
	}
}

// RFC 1576 clause 2: terminal type, then binary and end of record.
func negotiation() [][]byte {
	return [][]byte{
		{iac, tnDo, optTType},
		{iac, tnWill, optTType},
		{iac, tnSB, optTType, 1, iac, tnSE},
		append(append([]byte{iac, tnSB, optTType, 0}, "IBM-3278-2-E"...), iac, tnSE),
		{iac, tnDo, optEOR, iac, tnWill, optEOR, iac, tnDo, optBin, iac, tnWill, optBin},
		{iac, tnWill, optEOR, iac, tnDo, optEOR, iac, tnWill, optBin, iac, tnDo, optBin},
	}
}

const addressCodes = "\x40\xC1\xC2\xC3\xC4\xC5\xC6\xC7\xC8\xC9\x4A\x4B\x4C\x4D\x4E\x4F" +
	"\x50\xD1\xD2\xD3\xD4\xD5\xD6\xD7\xD8\xD9\x5A\x5B\x5C\x5D\x5E\x5F" +
	"\x60\x61\xE2\xE3\xE4\xE5\xE6\xE7\xE8\xE9\x6A\x6B\x6C\x6D\x6E\x6F" +
	"\xF0\xF1\xF2\xF3\xF4\xF5\xF6\xF7\xF8\xF9\x7A\x7B\x7C\x7D\x7E\x7F"

func sba(b []byte, row, col int) []byte {
	a := row*cols + col
	return append(b, orderSBA, addressCodes[a>>6&0x3F], addressCodes[a&0x3F])
}

func record(b []byte) []byte {
	return append(b, iac, tnEOR)
}

func formScreen() []byte {
	b := []byte{cmdEraseWrite, wcc}
	b = sba(b, 0, 1)
	b = append(b, orderSF, attrProtected)
	b = append(b, "SERVICE ORDER                                         EMCOMM DISPATCH"...)
	for i, f := range formFields() {
		b = sba(b, f.row, f.col-1)
		b = append(b, orderSF, attrProtected)
		b = append(b, f.label+":"...)
		b = append(b, orderSF, attrInput)
		if i == 0 {
			b = append(b, orderIC)
		}
		b = sba(b, f.row, f.col+len(f.label)+2+f.width)
		b = append(b, orderSF, attrProtected)
	}
	return record(b)
}

func formInput(rng *rand.Rand) []byte {
	streets := []string{"MAIN ST", "OAK AVE", "HWY 59", "FM 1960", "RIVER RD"}
	values := []string{
		fmt.Sprintf("SO%06d", rng.IntN(1000000)), fmt.Sprintf("MU%02d", rng.IntN(100)), "2",
		"NEED ASSIST", "14:05", fmt.Sprintf("%d %s", rng.IntN(9999), streets[rng.IntN(len(streets))]),
		"TOMBALL", "EL29FX", "J SMITH", "2815550100", "WATER", "40",
		"ROAD FLOODED AT CROSSING USE ALT ROUTE",
	}
	b := []byte{aidEnter, 0x40, 0x40}
	for i, f := range formFields() {
		b = sba(b, f.row, f.col+len(f.label)+2)
		b = append(b, values[i]...)
	}
	return record(b)
}

func listScreen(rng *rand.Rand) []byte {
	b := []byte{cmdEraseWrite, wcc}
	b = sba(b, 0, 1)
	b = append(b, orderSF, attrProtected)
	b = append(b, "OPEN ORDERS                                           EMCOMM DISPATCH"...)
	statuses := []string{"OPEN", "ENROUTE", "ON SCENE", "NEED ASSIST"}
	for row := 2; row < 23; row++ {
		b = sba(b, row, 0)
		b = append(b, orderSF, attrProtected)
		line := fmt.Sprintf("SO%06d MU%02d %-11s %4d %-20s %-10s %s", rng.IntN(1000000), rng.IntN(100),
			statuses[rng.IntN(len(statuses))], rng.IntN(9999), "RIVER RD", "CONROE", strings.Repeat("-", 8))
		b = append(b, line[:min(len(line), cols-2)]...)
	}
	return record(b)
}

func pf3() []byte {
	return record([]byte{aidPF3, 0x40, 0x40})
}
