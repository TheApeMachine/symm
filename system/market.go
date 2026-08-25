package system

import "github.com/spf13/viper"

type Market struct {
	Book *Book
}

type Book struct {
	Depth int
}

func NewMarket() *Market {
	viper.SetDefault("market.book.depth", 10)

	return &Market{
		Book: &Book{
			Depth: viper.GetInt("market.book.depth"),
		},
	}
}
