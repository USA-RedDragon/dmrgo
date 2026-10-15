package fec_test

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ExampleResult_Err turns an uncorrectable FEC result into an error that
// matches fec.ErrUncorrectable.
func ExampleResult_Err() {
	res := fec.Result{Uncorrectable: true}
	if err := res.Err(); errors.Is(err, fec.ErrUncorrectable) {
		fmt.Println(err)
	}
	// Output: fec: uncorrectable
}
