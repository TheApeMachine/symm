package adaptive

/*
PeerValues returns map values excluding skip for cross-section dynamics.
*/
func PeerValues(values map[string]float64, skip string) []float64 {
	peers := make([]float64, 0, len(values))

	for symbol, value := range values {
		if symbol == skip {
			continue
		}

		peers = append(peers, value)
	}

	return peers
}
