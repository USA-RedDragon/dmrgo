// Package allocgate holds tests that fail when the gc compiler or TinyGo
// finds a heap allocation in the library, outside the functions that its
// rules allow to allocate.
package allocgate
