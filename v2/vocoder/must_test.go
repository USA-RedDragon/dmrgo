package vocoder_test

// must returns v, and panics when err is not nil.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
