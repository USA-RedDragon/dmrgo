package fec_test

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

func ExampleResult_Err() {
	res := fec.Result{Uncorrectable: true}
	if err := res.Err(); errors.Is(err, fec.ErrUncorrectable) {
		fmt.Println(err)
	}
	// Output: fec: uncorrectable
}
