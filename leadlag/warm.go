package leadlag

import "github.com/theapemachine/symm/engine"

/*
WarmFromOHLC seeds return history and the cross-section volume leader.
*/
func (trackStore *TrackStore) WarmFromOHLC(candles map[string][]engine.OHLCCandle) {
	volumes := make(map[string]float64, len(candles))

	for symbol, bars := range candles {
		volumes[symbol] = trackStore.warmSymbol(symbol, bars)
	}

	leader := pickLeader(volumes)

	if leader == "" {
		return
	}

	trackStore.setLeader(leader)
}

func (trackStore *TrackStore) warmSymbol(symbol string, bars []engine.OHLCCandle) float64 {
	completed := engine.CompletedCandles(bars)

	if len(completed) == 0 {
		return 0
	}

	track := trackStore.ensure(symbol)
	totalVolume := 0.0

	for index, bar := range completed {
		totalVolume += bar.Volume

		if index == 0 {
			track.lastPrice = bar.Close
			track.hasLast = true

			continue
		}

		previous := completed[index-1]

		if previous.Close <= 0 {
			continue
		}

		ret := bar.Close/previous.Close - 1
		track.returns = append(track.returns, ret)

		if len(track.returns) > historyCap {
			track.returns = track.returns[len(track.returns)-historyCap:]
		}

		track.lastPrice = bar.Close
	}

	return totalVolume
}
