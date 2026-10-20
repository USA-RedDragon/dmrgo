//go:build !linux

package main

import (
	"errors"
	"os"
)

func openSerial(string, int) (*os.File, error) {
	return nil, errors.New("serial ports are supported on Linux only")
}
