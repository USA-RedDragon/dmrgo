// Command speccheck checks spec_matrix.yaml against the code. It runs the Go
// tests listed for each spec section and gives each section a status:
//
//   - verified: it has tests and all of them pass
//   - failing: a listed test fails or does not run
//   - untested: it has source files and no tests
//   - unimplemented: it has no source files and no tests
//
// It also checks that every source_files and ignore_files entry exists, and
// that every non-test, non-generated Go file under v2/ is in a section's
// source_files or in ignore_files. It prints a summary per document and
// exits non-zero on any error.
//
// Usage:
//
//	go run -C cmd/speccheck .
//	go run -C cmd/speccheck . -update-readme
//	go run -C cmd/speccheck . -check-readme
//
// -update-readme writes the Markdown report between the ETSI COVERAGE markers
// of README.md, and only when there are no errors. -check-readme fails when
// that section is out of date.
package main
