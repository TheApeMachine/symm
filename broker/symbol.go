package broker

import "fmt"

/*
Symbol is one Kraken wsname such as BTC/EUR.
*/
type Symbol string

/*
BaseAsset returns the asset prefix before the quote currency separator.
*/
func (symbol Symbol) BaseAsset() string {
	wsname := string(symbol)

	for index := range wsname {
		if wsname[index] == '/' {
			return wsname[:index]
		}
	}

	return wsname
}

/*
PaperOrderID builds one deterministic paper order id.
*/
func (symbol Symbol) PaperOrderID(kind string) string {
	return fmt.Sprintf("paper-%s-%s", kind, symbol)
}
