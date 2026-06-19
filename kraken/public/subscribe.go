package public

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/types"
)

/*
SetSymbols stores the discovered symbol universe for post-connect subscriptions.
*/
func (ws *WebSocket) SetSymbols(symbols []string) {
	if ws == nil {
		return
	}

	ws.symbols = append([]string(nil), symbols...)
}

func (ws *WebSocket) subscribeMarket() error {
	if ws == nil || ws.pool == nil || len(ws.symbols) == 0 {
		return nil
	}

	pace := viper.GetDuration("market.subscribe_pace")
	depth := viper.GetInt("market.book_depth_levels")

	if err := ws.sendSubscribeFrame(instrumentSubscribeParams()); err != nil {
		return err
	}

	if pace > 0 {
		time.Sleep(pace)
	}

	for _, batch := range symbolBatches(ws.symbols) {
		frames := []any{
			bookSubscribeParams(batch, depth),
			tradeSubscribeParams(batch),
			tickerSubscribeParams(batch),
		}

		for _, params := range frames {
			if err := ws.sendSubscribeFrame(params); err != nil {
				return err
			}

			if pace > 0 {
				time.Sleep(pace)
			}
		}
	}

	return nil
}

func (ws *WebSocket) sendSubscribeFrame(params any) error {
	message, buildErr := types.NewKrakenMessage("subscribe", params, 0)

	if buildErr != nil {
		return errnie.Error(buildErr)
	}

	payload, marshalErr := sonic.Marshal(message)

	if marshalErr != nil {
		return errnie.Error(marshalErr)
	}

	artifact := datura.Acquire("public", datura.Artifact_Type_json).
		WithDestination("kraken:public").
		WithPayload(payload)

	return errnie.Error(
		ws.pool.CreateBroadcastGroup("kraken:public").Send(artifact),
	)
}

func symbolBatches(symbols []string) [][]string {
	batchSize := viper.GetInt("market.subscribe_batch")

	if batchSize <= 0 || len(symbols) <= batchSize {
		if len(symbols) == 0 {
			return nil
		}

		return [][]string{symbols}
	}

	batches := make([][]string, 0, (len(symbols)+batchSize-1)/batchSize)

	for start := 0; start < len(symbols); start += batchSize {
		end := start + batchSize

		if end > len(symbols) {
			end = len(symbols)
		}

		batches = append(batches, symbols[start:end])
	}

	return batches
}
