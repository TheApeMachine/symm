package system

import (
	"time"

	"github.com/spf13/viper"
)

type Market struct {
	Model         string
	QuoteCurrency string
	Balance       int
	Instrument    *Instrument
	Subscribe     *Subscribe
	Level3        *Level3
}

type Instrument struct {
	Excluded []string
}

type Subscribe struct {
	Batch int
	Pace  time.Duration
}

/*
Level3 is the venue book tape and subscription surface.
*/
type Level3 struct {
	Depth            int
	RingCapacity     int
	Record           bool
	RecordFlushBytes int
	RecordFlushLines int
}

func NewMarket() *Market {
	viper.SetDefault("market.model", "paper")
	viper.SetDefault("market.quote_currency", "USD")
	viper.SetDefault("market.instrument.excluded", []string{
		"USD", "EUR", "GBP", "AUD", "CAD", "CHF", "JPY", "NZD",
		"USDT", "USDC", "DAI", "PYUSD", "FDUSD", "TUSD", "USDG",
		"USDE", "EURT", "EURC", "GUSD", "BUSD", "FRAX", "LUSD",
		"CUSD", "USD0", "USDS", "RLUSD", "UST",
	})
	viper.SetDefault("market.balance", 200)
	viper.SetDefault("market.subscribe.batch", 100)
	viper.SetDefault("market.subscribe.pace", 500*time.Millisecond)
	// l3_depth is the subscribed Kraken L3 depth: the number of PRICE LEVELS
	// per side, not the number of individual orders. It stays in lockstep
	// with cmd/cfg/config.yml (market.l3_depth) so the execution reducer can
	// read the authoritative subscription depth without guessing.
	viper.SetDefault("market.l3_depth", 10)
	viper.SetDefault("market.l3_ring_capacity", 8192)
	viper.SetDefault("market.level3.record", true)
	viper.SetDefault("market.level3.record_flush_bytes", 4<<20)
	viper.SetDefault("market.level3.record_flush_lines", 2000)

	return &Market{
		Model:         viper.GetString("market.model"),
		QuoteCurrency: viper.GetString("market.quote_currency"),
		Balance:       viper.GetInt("market.balance"),
		Instrument: &Instrument{
			Excluded: viper.GetStringSlice("market.instrument.excluded"),
		},
		Subscribe: &Subscribe{
			Batch: viper.GetInt("market.subscribe.batch"),
			Pace:  viper.GetDuration("market.subscribe.pace"),
		},
		Level3: &Level3{
			Depth:            viper.GetInt("market.l3_depth"),
			RingCapacity:     viper.GetInt("market.l3_ring_capacity"),
			Record:           viper.GetBool("market.level3.record"),
			RecordFlushBytes: viper.GetInt("market.level3.record_flush_bytes"),
			RecordFlushLines: viper.GetInt("market.level3.record_flush_lines"),
		},
	}
}
