// Package dmrgo holds the go:generate directives of the repository: go
// generate ./... runs tablegen and then dmrgen, which write the _gen.go
// files of the v2 module, and then apigen, which writes the allapi program
// of the allocation gate from the exports of the library.
package dmrgo

//go:generate go run -C cmd/tablegen .
//go:generate go run -C cmd/dmrgen .
//go:generate go run -C v2 ./internal/allocgate/apigen/gen
