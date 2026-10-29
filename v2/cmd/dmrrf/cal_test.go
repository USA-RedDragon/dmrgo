package main

func decimate(x []complex128, rate float64) []complex128 {
	return boxcar(x, rate, basebandRate)
}

func boxcar(x []complex128, rate, to float64) []complex128 {
	ratio := rate / to
	n := int(float64(len(x)) / ratio)
	out := make([]complex128, n)
	for i := range n {
		lo := int(float64(i) * ratio)
		hi := min(int(float64(i+1)*ratio), len(x))
		var s complex128
		for _, v := range x[lo:hi] {
			s += v
		}
		out[i] = s / complex(float64(max(hi-lo, 1)), 0)
	}
	return out
}
