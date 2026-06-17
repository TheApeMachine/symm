package futures

import (
	"context"
	"fmt"
)

/*
Catalog maps spot pairs to related Kraken futures product ids.
*/
type Catalog struct {
	products map[string][]string
}

var sharedCatalog = &Catalog{
	products: map[string][]string{
		"XBT/USD": {"PI_XBTUSD"},
		"ETH/USD": {"PI_ETHUSD"},
	},
}

/*
SharedCatalog returns the process-wide futures product catalog.
*/
func SharedCatalog() *Catalog {
	return sharedCatalog
}

/*
EnsureLoaded marks the catalog ready for lookup.
*/
func (catalog *Catalog) EnsureLoaded(context.Context) error {
	return nil
}

/*
ProductsForSpotPair returns futures product ids linked to one spot pair.
*/
func (catalog *Catalog) ProductsForSpotPair(pair string) ([]string, error) {
	products, ok := catalog.products[pair]

	if !ok {
		return nil, fmt.Errorf("futures: no products for spot pair %q", pair)
	}

	copied := append([]string(nil), products...)

	return copied, nil
}
