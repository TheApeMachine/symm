package kraken

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/krakenfx/api-go/v2/pkg/derivatives"
	"github.com/theapemachine/errnie"
)

/* ProductsServer owns the venue catalogue and admitted spot-to-perpetual membership. */
type ProductsServer struct {
	symbols    map[string]bool
	catalogue  map[string]derivatives.Instrument
	subscribed map[string]bool
	changed    []string
}

/* NewProducts constructs an idle catalogue without network activity. */
func NewProducts() *ProductsServer {
	return &ProductsServer{symbols: make(map[string]bool), catalogue: make(map[string]derivatives.Instrument), subscribed: make(map[string]bool)}
}

/* Write joins eligible spot arrivals to the venue's own product specifications. */
func (server *ProductsServer) Write(ctx context.Context, call Products_write) error {
	payloads, err := call.Args().Instruments()

	if err != nil {
		return errnie.Error(err)
	}
	for index := range payloads.Len() {
		payload, err := payloads.At(index)

		if err != nil {
			return errnie.Error(err)
		}

		if err := server.index(payload); err != nil {
			return err
		}
	}
	symbols, err := call.Args().Symbols()

	if err != nil {
		return errnie.Error(err)
	}
	for index := range symbols.Len() {
		symbol, err := symbols.At(index)

		if err != nil {
			return errnie.Error(err)
		}
		server.symbols[symbol] = true
	}
	for product, listed := range server.catalogue {
		symbol := strings.ToUpper(strings.ReplaceAll(listed.Pair, ":", "/"))

		if !listed.Tradeable || !strings.HasPrefix(product, "PF_") || !server.symbols[symbol] || server.subscribed[product] {
			continue
		}
		server.subscribed[product] = true
		server.changed = append(server.changed, product)
	}
	return nil
}

/* index retains the authoritative venue fields from one instrument response. */
func (server *ProductsServer) index(payload []byte) error {
	var response struct {
		Result      string                   `json:"result"`
		Instruments []derivatives.Instrument `json:"instruments"`
	}

	if err := json.Unmarshal(payload, &response); err != nil {
		return errnie.Error(err)
	}

	if response.Result != "success" {
		return errnie.Error(errnie.Err(errnie.Validation, "products: instrument request did not succeed", nil))
	}
	for _, listed := range response.Instruments {
		product := strings.ToUpper(listed.Symbol)

		if product == "" || listed.Pair == "" {
			return errnie.Error(errnie.Err(errnie.Validation, "products: instrument symbol and pair are required", nil))
		}
		server.catalogue[product] = listed
	}
	return nil
}

/* Done emits new subscriptions once and the complete handshake for reconnects. */
func (server *ProductsServer) Done(ctx context.Context, call Products_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}

	if len(server.changed) == 0 {
		return nil
	}
	all := make([]string, 0, len(server.subscribed))
	for product := range server.subscribed {
		all = append(all, product)
	}
	sort.Strings(all)
	sort.Strings(server.changed)
	for index, products := range [][]string{server.changed, all} {
		var frames capnp.DataList
		var err error
		if index == 0 {
			frames, err = result.NewSubscribe(2)
		}

		if index == 1 {
			frames, err = result.NewOnConnect(2)
		}

		if err != nil {
			return errnie.Error(err)
		}
		for position, feed := range []string{"ticker", "trade"} {
			payload, err := json.Marshal(struct {
				Event    string   `json:"event"`
				Feed     string   `json:"feed"`
				Products []string `json:"product_ids"`
			}{"subscribe", feed, products})

			if err != nil {
				return errnie.Error(err)
			}

			if err := frames.Set(position, payload); err != nil {
				return errnie.Error(err)
			}
		}
	}
	server.changed = nil
	return nil
}

/* Lookup translates every known contract using its authoritative Pair field. */
func (server *ProductsServer) Lookup(ctx context.Context, call Products_lookup) error {
	product, err := call.Args().Product()

	if err != nil {
		return errnie.Error(err)
	}
	listed, found := server.catalogue[strings.ToUpper(product)]

	if !found {
		return errnie.Error(errnie.Err(errnie.Validation, "products: unknown futures product "+product, nil))
	}
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	return errnie.Error(result.SetSymbol(strings.ToUpper(strings.ReplaceAll(listed.Pair, ":", "/"))))
}
