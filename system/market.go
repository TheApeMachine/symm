package system

import "github.com/spf13/viper"

type Market struct {
	Instrument *Instrument
	Book       *Book
}

type Instrument struct {
	Excluded []string
}

type Book struct {
	Depth int
}

func NewMarket() *Market {
	viper.SetDefault("market.instrument.excluded", []string{
		"USD", "EUR", "GBP", "AUD", "CAD", "CHF", "JPY", "NZD",
		"USDT", "USDC", "DAI", "PYUSD", "FDUSD", "TUSD", "USDG",
		"USDE", "EURT", "EURC", "GUSD", "BUSD", "FRAX", "LUSD",
		"CUSD", "USD0", "USDS", "RLUSD", "UST",
	})
	viper.SetDefault("market.book.depth", 10)
	// l3_depth is the subscribed Kraken L3 depth: the number of PRICE LEVELS
	// per side, not the number of individual orders. It stays in lockstep
	// with cmd/cfg/config.yml (market.l3_depth) so the execution reducer can
	// read the authoritative subscription depth without guessing.
	viper.SetDefault("market.l3_depth", 10)

	return &Market{
		Instrument: &Instrument{
			Excluded: viper.GetStringSlice("market.instrument.excluded"),
		},
		Book: &Book{
			Depth: viper.GetInt("market.book.depth"),
		},
	}
}
