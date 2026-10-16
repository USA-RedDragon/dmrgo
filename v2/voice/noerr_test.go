package voice_test

// noErr panics when err is not nil, for a call that a test expects to
// succeed.
func noErr(err error) {
	if err != nil {
		panic(err)
	}
}
