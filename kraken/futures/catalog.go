package futures

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

const instrumentsURL = "https://futures.kraken.com/derivatives/api/v3/instruments"

/*
Catalog maps spot bases to Kraken derivatives product ids (perpetuals and dated futures).
*/
type Catalog struct {
	mu      sync.RWMutex
	loaded  bool
	loadErr error
	byBase  map[string][]string
	client  *http.Client
}

var sharedCatalog = &Catalog{
	client: &http.Client{Timeout: 15 * time.Second},
	byBase: make(map[string][]string),
}

/*
SharedCatalog returns the process-wide futures instrument catalog.
*/
func SharedCatalog() *Catalog {
	return sharedCatalog
}

/*
EnsureLoaded fetches instruments once and indexes tradeable PI_/FI_ products by base asset.
*/
func (catalog *Catalog) EnsureLoaded(ctx context.Context) error {
	catalog.mu.RLock()

	if catalog.loaded {
		err := catalog.loadErr
		catalog.mu.RUnlock()
		return err
	}

	catalog.mu.RUnlock()

	catalog.mu.Lock()
	defer catalog.mu.Unlock()

	if catalog.loaded {
		return catalog.loadErr
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, instrumentsURL, nil)

	if err != nil {
		catalog.loadErr = err
		catalog.loaded = true
		return err
	}

	response, err := catalog.client.Do(request)

	if err != nil {
		catalog.loadErr = err
		catalog.loaded = true
		return err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		catalog.loadErr = fmt.Errorf("futures: instruments status %s", response.Status)
		catalog.loaded = true
		return catalog.loadErr
	}

	payload, err := io.ReadAll(response.Body)

	if err != nil {
		catalog.loadErr = err
		catalog.loaded = true
		return err
	}

	catalog.loadErr = catalog.parseInstruments(payload)
	catalog.loaded = true

	return catalog.loadErr
}

/*
ProductsForSpotPair returns PI_ and FI_ product ids for a USD spot pair base asset.
*/
func (catalog *Catalog) ProductsForSpotPair(symbol string) ([]string, error) {
	spotIdentity, err := market.SpotIdentityFromPair(symbol)

	if err != nil {
		return nil, err
	}

	_, quote, splitErr := market.SplitPairSymbol(symbol)

	if splitErr != nil {
		return nil, splitErr
	}

	if strings.ToUpper(quote) != "USD" {
		return nil, fmt.Errorf("futures: catalog requires USD quote for %q", symbol)
	}

	catalog.mu.RLock()
	defer catalog.mu.RUnlock()

	if catalog.loadErr != nil {
		return nil, catalog.loadErr
	}

	products := append([]string(nil), catalog.byBase[spotIdentity.Base]...)

	return products, nil
}

func (catalog *Catalog) parseInstruments(payload []byte) error {
	var envelope struct {
		Result      string `json:"result"`
		Instruments []struct {
			Symbol    string `json:"symbol"`
			Tradeable bool   `json:"tradeable"`
			IsExpired bool   `json:"isExpired"`
		} `json:"instruments"`
	}

	if err := json.Unmarshal(payload, &envelope); err != nil {
		return err
	}

	if envelope.Result != "success" {
		return fmt.Errorf("futures: instruments result %q", envelope.Result)
	}

	byBase := make(map[string][]string)

	for _, row := range envelope.Instruments {
		if !row.Tradeable || row.IsExpired {
			continue
		}

		productID := strings.ToUpper(strings.TrimSpace(row.Symbol))

		if !strings.HasPrefix(productID, "PI_") && !strings.HasPrefix(productID, "FI_") {
			continue
		}

		identity, identityErr := market.FuturesIdentityFromProduct(productID)

		if identityErr != nil {
			continue
		}

		byBase[identity.Base] = append(byBase[identity.Base], productID)
	}

	catalog.byBase = byBase

	return nil
}
