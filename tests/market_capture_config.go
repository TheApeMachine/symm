package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/theapemachine/symm/kraken"
	tes "github.com/theapemachine/symm/tests/types"
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

type capturedStart struct {
	last float64
	bid  float64
	ask  float64
}

/*
CaptureSymbolsFromFrames reconstructs replay configuration from one live
capture containing market profiles and ticker frames.
*/
func CaptureSymbolsFromFrames(
	reader io.Reader,
	depth int,
) ([]*tes.Symbol, error) {
	if reader == nil || depth <= 0 {
		return nil, fmt.Errorf("market: capture and positive depth required")
	}

	decoder := json.NewDecoder(reader)
	profiles := make(map[string]kraken.MarketProfile)
	starts := make(map[string]capturedStart)

	for record := 1; ; record++ {
		frame := captureFrame{}
		err := decoder.Decode(&frame)

		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("market: decode capture record %d: %w", record, err)
		}

		payload := struct {
			Channel string                 `json:"channel"`
			Type    string                 `json:"type"`
			Data    []kraken.MarketProfile `json:"data"`
		}{}

		if err = json.Unmarshal(frame.Payload, &payload); err != nil {
			return nil, fmt.Errorf("market: decode capture payload %d: %w", record, err)
		}

		if payload.Channel == "symm_metadata" && payload.Type == "market_profiles" {
			for _, profile := range payload.Data {
				profiles[profile.Symbol] = profile
			}

			continue
		}

		if payload.Channel != "ticker" {
			continue
		}

		ticker := struct {
			Data []struct {
				Symbol string      `json:"symbol"`
				Last   json.Number `json:"last"`
				Bid    json.Number `json:"bid"`
				Ask    json.Number `json:"ask"`
			} `json:"data"`
		}{}

		if err = json.Unmarshal(frame.Payload, &ticker); err != nil {
			return nil, fmt.Errorf("market: decode ticker payload %d: %w", record, err)
		}

		for _, data := range ticker.Data {
			if _, found := starts[data.Symbol]; found {
				continue
			}

			last, lastErr := data.Last.Float64()
			bid, bidErr := data.Bid.Float64()
			ask, askErr := data.Ask.Float64()

			if lastErr != nil || bidErr != nil || askErr != nil ||
				last <= 0 || bid <= 0 || ask <= bid {
				return nil, fmt.Errorf("market: invalid first ticker for %s", data.Symbol)
			}

			starts[data.Symbol] = capturedStart{last: last, bid: bid, ask: ask}
		}
	}

	names := make([]string, 0, len(profiles))

	for symbol := range profiles {
		if _, found := starts[symbol]; found {
			names = append(names, symbol)
		}
	}

	sort.Strings(names)
	symbols := make([]*tes.Symbol, 0, len(names))

	for index, name := range names {
		profile := profiles[name]
		start := starts[name]
		pair := profile.Pair

		if pair.TickSize == nil || pair.OrderMinimum == nil ||
			pair.CostMinimum == nil || profile.Taker.Fee == nil ||
			profile.Maker.Fee == nil {
			return nil, fmt.Errorf("market: incomplete profile for %s", name)
		}

		midpoint := (start.ask + start.bid) / 2
		symbol := tes.NewSymbol(name, start.last, int64(index+1))
		symbol.PriceIncrement = pair.TickSize.Float64()
		symbol.PricePrecision = pair.PairDecimals
		symbol.QuantityPrecision = pair.LotDecimals
		symbol.BaseSpreadFraction = (start.ask - start.bid) / midpoint
		symbol.TakerFeePercent = profile.Taker.Fee.Float64()
		symbol.MakerFeePercent = profile.Maker.Fee.Float64()
		symbol.OrderMinimum = pair.OrderMinimum.Float64()
		symbol.CostMinimum = pair.CostMinimum.Float64()
		symbol.BookDepthLevels = depth
		symbols = append(symbols, symbol)
	}

	if len(symbols) == 0 {
		return nil, fmt.Errorf("market: capture has no complete market profiles")
	}

	return symbols, nil
}

/*
CaptureSymbols reconstructs replay instrument configuration from the captured
Kraken pair response and the first observed ticker mark for every USD symbol.
*/
func CaptureSymbols(
	pairs io.Reader,
	tickers io.Reader,
	depth int,
) ([]*tes.Symbol, error) {
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
	symbols := make([]*tes.Symbol, 0, len(names))

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

		symbol := tes.NewSymbol(name, prices[name], int64(index+1))
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
