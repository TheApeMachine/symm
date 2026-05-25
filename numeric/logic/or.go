package logic

func Or[T any](a, b T, is bool) T {
	if is {
		return a
	}

	return b
}
