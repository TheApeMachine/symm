package broker

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
)

const tickerRefresh = time.Minute

type tickerSnap struct {
	volumeUSD float64
	changePct float64
}

// SetTicker stores one pair's public Ticker 24h quote volume and day change.
func (price *Price) SetTicker(symbol string, volumeUSD, changePct float64) {
	if price == nil || symbol == "" {
		return
	}

	price.ticker.Store(symbol, tickerSnap{volumeUSD, changePct})
}

// VolumeUSD is the last Ticker 24h quote notional for the symbol.
func (price *Price) VolumeUSD(symbol string) float64 {
	if price == nil || symbol == "" {
		return 0
	}

	raw, ok := price.ticker.Load(symbol)
	if !ok {
		return 0
	}

	return raw.(tickerSnap).volumeUSD
}

// ChangePct is the last Ticker open-to-last percent move for the symbol.
func (price *Price) ChangePct(symbol string) float64 {
	if price == nil || symbol == "" {
		return 0
	}

	raw, ok := price.ticker.Load(symbol)
	if !ok {
		return 0
	}

	return raw.(tickerSnap).changePct
}

func (price *Price) refreshTicker() {
	if price == nil {
		return
	}

	path := "/0/public/Ticker"

	if system.Cfg != nil && system.Cfg.WebSocket != nil &&
		system.Cfg.WebSocket.Endpoints != nil &&
		system.Cfg.WebSocket.Endpoints.Ticker != "" {
		path = system.Cfg.WebSocket.Endpoints.Ticker
	}

	response, err := network.NewREST().Get(
		"https://api.kraken.com"+path,
		map[string]string{"assetVersion": "1"},
	)

	if err != nil {
		errnie.Warn("[price] ticker poll failed", "err", err)
		return
	}

	rows, err := kraken.ParseTicker(response.Body())
	if err != nil {
		errnie.Warn("[price] ticker parse failed", "err", err)
		return
	}

	for pair, row := range rows {
		symbol := pair

		if price.normalizer != nil {
			if name := price.normalizer.Name(pair); name != "" {
				symbol = name
			}
		}

		price.SetTicker(symbol, row.QuoteVolume24h(), row.ChangePct())
	}
}

func (price *Price) pollTicker() {
	if price == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(tickerRefresh)
		defer ticker.Stop()

		for {
			select {
			case <-price.Context().Done():
				return
			case <-ticker.C:
				price.refreshTicker()
			}
		}
	}()
}
