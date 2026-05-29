package pumpdump

import (
	"errors"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
)

var ErrNoVolumeData = errors.New("no valid volume data")

/*
WarmHourlyVolumeBaseline loads median hourly volume from Kraken OHLC history.
*/
func WarmHourlyVolumeBaseline(pair asset.Pair) (float64, error) {
	candles, err := rest.FetchOHLC(pair.Wsname, config.System.SlowRVOLIntervalMinutes)

	if err != nil {
		return 0, err
	}

	if len(candles) == 0 {
		return 0, ErrNoVolumeData
	}

	volumes := make([]float64, 0, len(candles))

	for _, candle := range candles {
		if candle.Volume > 0 {
			volumes = append(volumes, candle.Volume)
		}
	}

	if len(volumes) == 0 {
		return 0, ErrNoVolumeData
	}

	return numeric.PercentileSorted(numeric.CopySorted(volumes), 0.5), nil
}
