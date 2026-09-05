package manifold

type orderIdentity struct {
	symbol  string
	orderID string
}

/*
orderContentID is the stable identity of one venue order, folded from its
symbol and order id. It is what the physics domain merges and evicts on, so an
order keeps the particle it has been evolving for as long as it rests.

There is no order lifecycle here any more. The venue's book is the authority on
what is resting; residency is the difference between what it shows and what the
domain holds, and a second lifecycle rebuilt from a message tape could only
disagree with the first.
*/
func orderContentID(identity orderIdentity) int64 {
	const (
		offsetBasis = uint64(14695981039346656037)
		prime       = uint64(1099511628211)
		positive    = ^uint64(0) >> 1
	)

	hash := offsetBasis
	fold := func(value string) {
		length := uint64(len(value))

		for shift := 0; shift < 64; shift += 8 {
			hash ^= uint64(byte(length >> shift))
			hash *= prime
		}

		for index := 0; index < len(value); index++ {
			hash ^= uint64(value[index])
			hash *= prime
		}
	}

	fold(identity.symbol)
	fold(identity.orderID)

	return int64(hash & positive)
}
