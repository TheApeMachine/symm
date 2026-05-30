package sentiment

func (sentiment *Sentiment) marketBreadth() (float64, float64, bool) {
	positive := 0
	total := 0
	topChange := 0.0

	sentiment.symbols.Range(func(key, value any) bool {
		state := value.(*symbolState)
		snapshot := state.snapshot()

		if snapshot.changePct == 0 {
			return true
		}

		total++

		if snapshot.changePct > topChange {
			topChange = snapshot.changePct
		}

		if snapshot.changePct <= 0 {
			return true
		}

		positive++

		return true
	})

	if total == 0 {
		return 0, 0, false
	}

	return float64(positive) / float64(total), topChange, true
}

func (sentiment *Sentiment) breadthAndLeaders() (float64, map[string]float64, float64, bool) {
	breadth, topChange, ok := sentiment.marketBreadth()

	if !ok {
		return 0, nil, 0, false
	}

	leaders := make(map[string]float64)

	sentiment.symbols.Range(func(key, value any) bool {
		state := value.(*symbolState)
		snapshot := state.snapshot()

		if snapshot.changePct <= 0 {
			return true
		}

		leaders[key.(string)] = snapshot.changePct

		return true
	})

	if len(leaders) == 0 {
		return breadth, nil, topChange, true
	}

	if breadth < minBreadth || topChange <= 0 {
		return breadth, leaders, topChange, true
	}

	return breadth, leaders, topChange, true
}

func leaderPeers(leaders map[string]float64, skip string) []float64 {
	peers := make([]float64, 0, len(leaders)-1)

	for symbol, value := range leaders {
		if symbol == skip {
			continue
		}

		peers = append(peers, value)
	}

	return peers
}
