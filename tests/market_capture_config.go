package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	testtypes "github.com/theapemachine/symm/tests/types"
)

type capturedPair struct {
	WSName       string          `json:"wsname"`
	PairDecimals int             `json:"pair_decimals"`
	LotDecimals  int             `json:"lot_decimals"`
	Fees         [][]json.Number `json:"fees"`
	FeesMaker    [][]json.Number `json:"fees_maker"`
	OrderMinimum json.Number     `json:"ordermin"`
	CostMinimum  json.Number     `json:"costmin"`
	TickSize     json.Number     `json:"tick_size"`
	Status       string          `json:"status"`
}

/*
CaptureSymbols reconstructs replay instrument configuration from the captured
Kraken pair response and the first observed ticker mark for every USD symbol.
*/
func CaptureSymbols(
	pairs io.Reader,
	tickers io.Reader,
	depth int,
) ([]*testtypes.Symbol, error) {
	if pairs == nil || tickers == nil || depth <= 0 {
		return nil, fmt.Errorf("market: capture metadata and positive depth required")
	}

	pairMetadata := make(map[string]capturedPair)

	if err := json.NewDecoder(pairs).Decode(&pairMetadata); err != nil {
		return nil, fmt.Errorf("market: decode captured pairs: %w", err)
	}

	prices, err := captureStartPrices(tickers)

	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(pairMetadata))
	pairsByName := make(map[string]capturedPair, len(pairMetadata))

	for _, pair := range pairMetadata {
		if pair.Status != "online" || !strings.HasSuffix(pair.WSName, "/USD") {
			continue
		}

		if _, found := prices[pair.WSName]; !found {
			continue
		}

		names = append(names, pair.WSName)
		pairsByName[pair.WSName] = pair
	}

	sort.Strings(names)
	symbols := make([]*testtypes.Symbol, 0, len(names))

	for index, name := range names {
		pair := pairsByName[name]
		tickSize, err := pair.TickSize.Float64()

		if err != nil {
			return nil, fmt.Errorf("market: %s tick size: %w", name, err)
		}

		orderMinimum, err := pair.OrderMinimum.Float64()

		if err != nil {
			return nil, fmt.Errorf("market: %s order minimum: %w", name, err)
		}

		costMinimum, err := pair.CostMinimum.Float64()

		if err != nil {
			return nil, fmt.Errorf("market: %s cost minimum: %w", name, err)
		}

		takerFee, err := capturedBaseFee(pair.Fees)

		if err != nil {
			return nil, fmt.Errorf("market: %s taker fee: %w", name, err)
		}

		makerFee, err := capturedBaseFee(pair.FeesMaker)

		if err != nil {
			return nil, fmt.Errorf("market: %s maker fee: %w", name, err)
		}

		symbol := testtypes.NewSymbol(name, prices[name], int64(index+1))
		symbol.PriceIncrement = tickSize
		symbol.PricePrecision = pair.PairDecimals
		symbol.QuantityPrecision = pair.LotDecimals
		symbol.TakerFeePercent = takerFee
		symbol.MakerFeePercent = makerFee
		symbol.OrderMinimum = orderMinimum
		symbol.CostMinimum = costMinimum
		symbol.BookDepthLevels = depth
		symbols = append(symbols, symbol)
	}

	if len(symbols) == 0 {
		return nil, fmt.Errorf("market: capture has no configured USD symbols")
	}

	return symbols, nil
}

func captureStartPrices(reader io.Reader) (map[string]float64, error) {
	decoder := json.NewDecoder(reader)
	prices := make(map[string]float64)

	for record := 1; ; record++ {
		frame := captureFrame{}
		err := decoder.Decode(&frame)

		if errors.Is(err, io.EOF) {
			return prices, nil
		}

		if err != nil {
			return nil, fmt.Errorf("market: decode captured ticker %d: %w", record, err)
		}

		payload := struct {
			Channel string `json:"channel"`
			Data    []struct {
				Symbol string      `json:"symbol"`
				Last   json.Number `json:"last"`
			} `json:"data"`
		}{}

		if err = json.Unmarshal(frame.Payload, &payload); err != nil {
			return nil, fmt.Errorf("market: decode captured ticker payload %d: %w", record, err)
		}

		if payload.Channel != "ticker" {
			continue
		}

		for _, data := range payload.Data {
			if _, found := prices[data.Symbol]; found || data.Last == "" {
				continue
			}

			price, priceErr := data.Last.Float64()

			if priceErr != nil || price <= 0 {
				return nil, fmt.Errorf("market: invalid first ticker for %s", data.Symbol)
			}

			prices[data.Symbol] = price
		}
	}
}

func capturedBaseFee(schedule [][]json.Number) (float64, error) {
	for _, tier := range schedule {
		if len(tier) != 2 || tier[0] != "0" {
			continue
		}

		return tier[1].Float64()
	}

	return 0, fmt.Errorf("zero-volume fee tier required")
}
