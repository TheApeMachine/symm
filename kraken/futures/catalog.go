package futures

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

const instrumentsURL = "https://futures.kraken.com/derivatives/api/v3/instruments"

type catalogState struct {
	byBase  map[string][]string
	loadErr error
}

/*
Catalog maps spot bases to Kraken derivatives product ids (perpetuals and dated futures).
*/
type Catalog struct {
	loaded atomic.Bool
	state  atomic.Pointer[catalogState]
	client *http.Client
}

var sharedCatalog = &Catalog{
	client: &http.Client{Timeout: 15 * time.Second},
}

func init() {
	sharedCatalog.state.Store(&catalogState{
		byBase: make(map[string][]string),
	})
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
	if catalog.loaded.Load() {
		state := catalog.state.Load()

		if state != nil {
			return state.loadErr
		}

		return nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, instrumentsURL, nil)

	if err != nil {
		catalog.storeLoadResult(nil, err)
		return err
	}

	response, err := catalog.client.Do(request)

	if err != nil {
		catalog.storeLoadResult(nil, err)
		return err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		loadErr := fmt.Errorf("futures: instruments status %s", response.Status)
		catalog.storeLoadResult(nil, loadErr)

		return loadErr
	}

	payload, err := io.ReadAll(response.Body)

	if err != nil {
		catalog.storeLoadResult(nil, err)
		return err
	}

	byBase, parseErr := parseInstrumentPayload(payload)
	catalog.storeLoadResult(byBase, parseErr)

	return parseErr
}

func (catalog *Catalog) storeLoadResult(byBase map[string][]string, loadErr error) {
	if !catalog.loaded.CompareAndSwap(false, true) {
		return
	}

	state := &catalogState{
		byBase:  byBase,
		loadErr: loadErr,
	}

	if state.byBase == nil {
		state.byBase = make(map[string][]string)
	}

	catalog.state.Store(state)
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

	state := catalog.state.Load()

	if state == nil {
		return nil, fmt.Errorf("futures: catalog is not loaded")
	}

	if state.loadErr != nil {
		return nil, state.loadErr
	}

	products := append([]string(nil), state.byBase[spotIdentity.Base]...)

	return products, nil
}

func parseInstrumentPayload(payload []byte) (map[string][]string, error) {
	var envelope struct {
		Result      string `json:"result"`
		Instruments []struct {
			Symbol    string `json:"symbol"`
			Tradeable bool   `json:"tradeable"`
			IsExpired bool   `json:"isExpired"`
		} `json:"instruments"`
	}

	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, err
	}

	if envelope.Result != "success" {
		return nil, fmt.Errorf("futures: instruments result %q", envelope.Result)
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

	return byBase, nil
}
