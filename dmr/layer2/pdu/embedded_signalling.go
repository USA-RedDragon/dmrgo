package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/enums"
	quadraticResidue "github.com/USA-RedDragon/dmrgo/dmr/fec/quadratic_residue"
)

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.1.2 Embedded signalling (EMB) PDU
type EmbeddedSignalling struct {
	ColorCode                          int
	PreemptionAndPowerControlIndicator bool
	LCSS                               enums.LCSS
	ParityOK                           bool
	CorrectedErrors                    int
	Uncorrectable                      bool
}

func NewEmbeddedSignallingFromBits(data [16]bit.Bit) EmbeddedSignalling {
	es := EmbeddedSignalling{}

	corrected, errs, uncorrectable := quadraticResidue.Decode(data)
	es.CorrectedErrors = errs
	es.Uncorrectable = uncorrectable

	if !uncorrectable {
		data = corrected
	}
	es.ParityOK = (errs == 0)

	// Convert the first 4 bits of data into an int
	for i := 0; i < 4; i++ {
		if data[i] == 1 {
			es.ColorCode |= 1 << (3 - i)
		}
	}

	es.PreemptionAndPowerControlIndicator = data[4] == 1

	var linkControlStartStop int
	for i := 5; i < 7; i++ {
		if data[i] == 1 {
			linkControlStartStop |= 1 << (6 - i)
		}
	}

	es.LCSS = enums.LCSSFromInt(linkControlStartStop)

	return es
}

func (es *EmbeddedSignalling) Encode() [16]bit.Bit {
	var data [16]bit.Bit

	// Color Code (4 bits)
	for i := 0; i < 4; i++ {
		if (es.ColorCode>>(3-i))&1 == 1 {
			data[i] = 1
		}
	}

	// Preemption and Power Control Indicator (1 bit)
	if es.PreemptionAndPowerControlIndicator {
		data[4] = 1
	}

	// LCSS (2 bits)
	lcss := int(es.LCSS)
	if (lcss>>1)&1 == 1 {
		data[5] = 1
	}
	if (lcss & 1) == 1 {
		data[6] = 1
	}

	// Parity (9 bits)
	var shortData [7]bit.Bit
	copy(shortData[:], data[:7])
	parity := quadraticResidue.ParityBits(shortData)

	for i := 0; i < 9; i++ {
		data[7+i] = parity[i]
	}

	return data
}

// ToString returns a string representation of the EmbeddedSignalling
func (es EmbeddedSignalling) ToString() string {
	return fmt.Sprintf("{ Color Code: %d, Preemption and Power Control Indicator: %t, LCSS: %s, Parity OK: %t, Corrected: %d, Uncorrectable: %t }", es.ColorCode, es.PreemptionAndPowerControlIndicator, enums.LCSSToName(es.LCSS), es.ParityOK, es.CorrectedErrors, es.Uncorrectable)
}
