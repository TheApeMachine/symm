package broker

import (
	"fmt"
	"strings"
)

/*
Symbol is one Kraken wsname such as BTC/EUR.
*/
type Symbol string

/*
BaseAsset returns the asset prefix before the quote currency separator.
*/
func (symbol Symbol) BaseAsset() string {
	base, _, _ := strings.Cut(string(symbol), "/")

	return base
}

/*
PaperOrderID builds one deterministic paper order id.
*/
func (symbol Symbol) PaperOrderID(kind string) string {
	return fmt.Sprintf("paper-%s-%s", kind, symbol)
}
