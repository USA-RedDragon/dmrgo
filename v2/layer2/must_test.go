package layer2_test

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func mustSet(err error) {
	if err != nil {
		panic(err)
	}
}
