package sms

type noCopy struct{}

// Lock is a no-op that makes go vet's copylocks check see noCopy.
func (*noCopy) Lock() {}

// Unlock is a no-op that makes go vet's copylocks check see noCopy.
func (*noCopy) Unlock() {}
