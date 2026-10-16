package layer2

// noCopy marks a type that must not be copied after first use, because it
// keeps pointers into its own storage. go vet's copylocks check reports a
// copy of a type with a noCopy field, as it does for sync.Mutex. It has no
// size.
type noCopy struct{}

// Lock is a no-op that makes go vet's copylocks check see noCopy.
func (*noCopy) Lock() {}

// Unlock is a no-op that makes go vet's copylocks check see noCopy.
func (*noCopy) Unlock() {}
