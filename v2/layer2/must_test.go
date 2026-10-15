package layer2_test

// must returns v and panics on err, for encode calls a test expects to succeed.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
