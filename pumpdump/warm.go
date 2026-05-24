package pumpdump

import (
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/stats"
)

/*
WarmFromOHLC seeds rolling volume and price-move history from 5-minute candles.
*/
func (trackStore *TrackStore) WarmFromOHLC(candles map[string][]engine.OHLCCandle) {
	for symbol, bars := range candles {
		trackStore.warmSymbol(symbol, bars)
	}
}

func (trackStore *TrackStore) warmSymbol(symbol string, bars []engine.OHLCCandle) {
	completed := engine.CompletedCandles(bars)

	if len(completed) == 0 {
		return
	}

	track := trackStore.ensure(symbol)

	for _, bar := range completed {
		if bar.Volume > 0 {
			track.volumes = append(track.volumes, bar.Volume)

			if len(track.volumes) > volumeHistoryCap {
				track.volumes = track.volumes[len(track.volumes)-volumeHistoryCap:]
			}
		}

		if bar.Open > 0 && bar.Close > 0 {
			move := stats.AbsRelativeMove(bar.Close, bar.Open)
			track.priceMoves = append(track.priceMoves, move)

			if len(track.priceMoves) > priceHistoryCap {
				track.priceMoves = track.priceMoves[len(track.priceMoves)-priceHistoryCap:]
			}
		}
	}

	last := completed[len(completed)-1]
	track.lastPrice = last.Close

	recentVolume := 0.0

	for index := len(completed) - 1; index >= 0 && index >= len(completed)-12; index-- {
		recentVolume += completed[index].Volume
	}

	track.dailyQuoteVol = recentVolume * last.Close
}
