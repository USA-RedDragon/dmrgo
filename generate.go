// Package dmrgo holds the go:generate directives of the repository: go
// generate ./... runs tablegen and then dmrgen, which write the _gen.go
// files of the v2 module.
package dmrgo

//go:generate go run -C cmd/tablegen .
//go:generate go run -C cmd/dmrgen .
