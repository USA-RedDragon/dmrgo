package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/enums"
	quadraticResidue "github.com/USA-RedDragon/dmrgo/dmr/fec/quadratic_residue"
)

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.1.2 Embedded signalling (EMB) PDU
type EmbeddedSignalling struct {
	ColorCode                          int
	PreemptionAndPowerControlIndicator bool
	LCSS                               enums.LCSS
	ParityOK                           bool
}

func NewEmbeddedSignallingFromBits(data [16]byte) EmbeddedSignalling {
	es := EmbeddedSignalling{}
	// Convert the first 4 bits of data into an int
	for i := 0; i < 4; i++ {
		if data[i] == 1 {
			es.ColorCode |= 1 << uint(3-i)
		}
	}

	es.PreemptionAndPowerControlIndicator = data[4] == 1

	var linkControlStartStop int
	for i := 5; i < 7; i++ {
		if data[i] == 1 {
			linkControlStartStop |= 1 << uint(7-i)
		}
	}

	es.LCSS = enums.LCSSFromInt(linkControlStartStop)

	parity := 0
	for i := 7; i < 16; i++ {
		if data[i] == 1 {
			parity |= 1 << uint(15-i)
		}
	}

	// No parity found, so calculate it
	if parity <= 0 {
		parity = 0
		shortData := [7]byte{}
		copy(shortData[:], data[:7])
		// Loop through the returned array of fec.ParityBits
		// and convert them to an int
		for i, v := range quadraticResidue.ParityBits(shortData) {
			if v == 1 {
				parity |= 1 << uint(15-i)
			}
		}
	}

	if !quadraticResidue.Check(data) {
		fmt.Printf("Parity check failed\n")
		fmt.Println("Expected: ", data[7:16])
		bits := [7]byte{}
		copy(bits[:], data[:7])
		fmt.Println("Got:      ", quadraticResidue.ParityBits(bits))
		es.ParityOK = false
	} else {
		es.ParityOK = true
	}

	return es
}

// ToString returns a string representation of the EmbeddedSignalling
func (es EmbeddedSignalling) ToString() string {
	return fmt.Sprintf("{ Color Code: %d, Preemption and Power Control Indicator: %t, LCSS: %s, Parity OK: %t }", es.ColorCode, es.PreemptionAndPowerControlIndicator, enums.LCSSToName(es.LCSS), es.ParityOK)
}
