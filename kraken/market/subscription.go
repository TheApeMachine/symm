package market

var krakenBookDepths = []int{10, 25, 100, 500, 1000}

func closed[T any]() <-chan *T {
	out := make(chan *T)
	close(out)

	return out
}

func validBookDepth(depth int) int {
	for _, allowed := range krakenBookDepths {
		if depth <= allowed {
			return allowed
		}
	}

	return krakenBookDepths[len(krakenBookDepths)-1]
}
