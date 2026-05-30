package market

// krakenBookDepths is the set of order-book depths the Kraken v2 book channel
// accepts; any other value is rejected and the subscription returns nothing.
var krakenBookDepths = []int{10, 25, 100, 500, 1000}

/*
closed returns an already-closed channel of *T. Subscription constructors return
it on a setup failure so the caller's range terminates immediately instead of
blocking on a nil channel.
*/
func closed[T any]() <-chan *T {
	out := make(chan *T)
	close(out)
	return out
}

/*
validBookDepth snaps an arbitrary depth to the smallest Kraken book depth that
covers it, so a config value Kraken would reject still yields a live book.
*/
func validBookDepth(depth int) int {
	for _, allowed := range krakenBookDepths {
		if depth <= allowed {
			return allowed
		}
	}

	return krakenBookDepths[len(krakenBookDepths)-1]
}
