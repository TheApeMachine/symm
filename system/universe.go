package system

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
)

/*
InstrumentPair carries Kraken pair properties reported in the instrument channel.
*/
type InstrumentPair struct {
	Symbol string `json:"symbol"`
	Base   string `json:"base"`
	Quote  string `json:"quote"`
	Status string `json:"status"`
}

/*
InstrumentSnapshotData holds the list of instrument pairs from the snapshot.
*/
type InstrumentSnapshotData struct {
	Pairs []InstrumentPair `json:"pairs"`
}

/*
InstrumentSnapshotMessage matches the Kraken v2 instrument channel snapshot.
*/
type InstrumentSnapshotMessage struct {
	Channel string                 `json:"channel"`
	Type    string                 `json:"type"`
	Data    InstrumentSnapshotData `json:"data"`
}

/*
DiscoverUniverse dials the public Kraken WebSocket endpoint, subscribes to the
"instrument" channel, and awaits the snapshot frame. It extracts all listed pairs,
filtering for quote currency match, "online" status, and non-membership in excludedBases.
*/
func DiscoverUniverse(
	ctx context.Context,
	endpoint string,
	quoteCurrency string,
	excludedBases []string,
) ([]string, error) {
	return DiscoverUniverseWithDialer(ctx, websocket.DefaultDialer, endpoint, quoteCurrency, excludedBases)
}

/*
DiscoverUniverseWithDialer executes universe discovery using the provided websocket.Dialer.
*/
func DiscoverUniverseWithDialer(
	ctx context.Context,
	dialer *websocket.Dialer,
	endpoint string,
	quoteCurrency string,
	excludedBases []string,
) ([]string, error) {
	errnie.Info(fmt.Sprintf("[universe] querying %s for %s instruments...", endpoint, quoteCurrency))

	conn, _, err := dialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"[universe] dial failed: "+endpoint,
			err,
		))
	}
	defer conn.Close()

	subMsg := map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "instrument",
		},
	}

	payload, err := json.Marshal(subMsg)
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[universe] failed to marshal subscription",
			err,
		))
	}

	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"[universe] failed to write subscription",
			err,
		))
	}

	excludedMap := make(map[string]bool, len(excludedBases))
	for _, base := range excludedBases {
		excludedMap[strings.ToUpper(strings.TrimSpace(base))] = true
	}

	targetQuote := strings.ToUpper(strings.TrimSpace(quoteCurrency))
	timeout := time.After(10 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, errnie.Error(errnie.Err(
				errnie.Timeout,
				"[universe] timeout waiting for instrument snapshot",
				nil,
			))
		default:
		}

		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"[universe] read error from instrument channel",
				err,
			))
		}

		var snapshot InstrumentSnapshotMessage
		if err := json.Unmarshal(msgBytes, &snapshot); err != nil {
			continue
		}

		if snapshot.Channel != "instrument" || snapshot.Type != "snapshot" {
			continue
		}

		var symbols []string
		for _, pair := range snapshot.Data.Pairs {
			if strings.ToUpper(pair.Quote) != targetQuote {
				continue
			}

			if strings.ToLower(pair.Status) != "online" {
				continue
			}

			if excludedMap[strings.ToUpper(pair.Base)] {
				continue
			}

			symbols = append(symbols, pair.Symbol)
		}

		slices.Sort(symbols)
		errnie.Info(fmt.Sprintf("[universe] discovered %d online %s trading pairs", len(symbols), targetQuote))
		return symbols, nil
	}
}
