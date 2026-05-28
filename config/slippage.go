package config

import (
	"github.com/theapemachine/symm/kraken/market"
)

/*
SlippageFill returns a depth-weighted VWAP fill when book levels are available.
Falls back to half-spread on last when depth cannot cover the order.
*/
func (cfg Config) SlippageFill(
	last, bid, ask float64,
	side string,
	extraBPS float64,
	quoteNotional float64,
	bidLevels, askLevels []market.BookLevel,
) float64 {
	var levels []market.BookLevel

	switch side {
	case "buy":
		levels = askLevels
	case "sell":
		levels = bidLevels
	default:
		return cfg.SlippagePrice(last, bid, ask, side, extraBPS)
	}

	if quoteNotional > 0 && len(levels) > 0 {
		fill := market.DepthFillVWAPSide(levels, quoteNotional, side)

		if fill > 0 {
			extra := fill * extraBPS / 10000

			switch side {
			case "buy":
				return fill + extra
			case "sell":
				return fill - extra
			}
		}
	}

	return cfg.SlippagePrice(last, bid, ask, side, extraBPS)
}

/*
SlippagePrice applies half-spread plus extra bps on exit (worse fill).
*/
func (cfg Config) SlippagePrice(
	last, bid, ask float64,
	side string,
	extraBPS float64,
) float64 {
	if last <= 0 {
		return last
	}

	halfSpread := 0.0

	if bid > 0 && ask > 0 && ask >= bid {
		halfSpread = (ask - bid) / 2
	}

	extra := last * extraBPS / 10000

	switch side {
	case "buy":
		return last + halfSpread + extra
	case "sell":
		return last - halfSpread - extra
	default:
		return last
	}
}
