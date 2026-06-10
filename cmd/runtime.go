package cmd

import (
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

/*
Runtime holds shared market state wired once at boot.
*/
type Runtime struct {
	Instruments *krakenmarket.InstrumentRegistry
	Books       *krakenmarket.BookStore
}

func NewRuntime() *Runtime {
	depth := viper.GetInt("market.book_depth_levels")

	return &Runtime{
		Instruments: krakenmarket.NewInstrumentRegistry(),
		Books:       krakenmarket.NewBookStore(depth),
	}
}
