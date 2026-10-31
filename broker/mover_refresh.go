package broker

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
)

/*
refreshMovers pulls GET /0/public/Ticker and writes 24h quote volume into
Mover. Best-effort: a failed poll leaves the previous snapshot in place.
*/
func (price *Price) refreshMovers() {
	if price == nil || price.Mover == nil {
		return
	}

	path := "/0/public/Ticker"

	if system.Cfg != nil && system.Cfg.WebSocket != nil &&
		system.Cfg.WebSocket.Endpoints != nil &&
		system.Cfg.WebSocket.Endpoints.Ticker != "" {
		path = system.Cfg.WebSocket.Endpoints.Ticker
	}

	rest := network.NewREST()
	response, err := rest.Get(
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

		price.Mover.Apply(symbol, row.QuoteVolume24h(), row.ChangePct())
	}
}

/*
pollMovers refreshes the mover on MoverRefresh until Price's context ends.
*/
func (price *Price) pollMovers() {
	if price == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(MoverRefresh)
		defer ticker.Stop()

		for {
			select {
			case <-price.Context().Done():
				return
			case <-ticker.C:
				price.refreshMovers()
			}
		}
	}()
}
