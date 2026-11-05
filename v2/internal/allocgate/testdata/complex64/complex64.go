package complex64

func bare(a, b complex64) complex64 { return a * b }

func bareAssign(a, b complex64) complex64 {
	a *= b
	return a
}

func bareQuo(a, b complex64) complex64 { return a / b }

func scalar(a complex64, x float32) complex64 { return a * complex(x, 0) }

func constant(a complex64) complex64 { return 2 * a }

func wide(a, b complex128) complex128 { return a * b }
