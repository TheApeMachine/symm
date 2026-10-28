package causal

func containsIndex(indices []int, nodeIndex int) bool {
	for _, skipIndex := range indices {
		if skipIndex == nodeIndex {
			return true
		}
	}

	return false
}
