package transport

import (
	"context"
	"encoding/json"
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
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}

	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, resp, err := dialer.DialContext(dialCtx, endpoint, nil)
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"universe: dial failed for "+endpoint,
			err,
		))
	}

	if resp != nil && resp.Body != nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			errnie.Error(errnie.Err(errnie.IO, "universe: close response body", closeErr))
		}
	}

	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			errnie.Error(errnie.Err(errnie.IO, "universe: close websocket conn", closeErr))
		}
	}()

	subMsg := map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "instrument",
		},
	}

	subPayload, err := json.Marshal(subMsg)
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"universe: marshal instrument subscribe message",
			err,
		))
	}

	if err := conn.WriteMessage(websocket.TextMessage, subPayload); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"universe: write instrument subscribe message",
			err,
		))
	}

	normalizedQuote := strings.ToUpper(strings.TrimSpace(quoteCurrency))
	excludedSet := make(map[string]struct{}, len(excludedBases))
	for _, base := range excludedBases {
		excludedSet[strings.ToUpper(strings.TrimSpace(base))] = struct{}{}
	}

	for {
		select {
		case <-dialCtx.Done():
			return nil, errnie.Error(errnie.Err(
				errnie.Timeout,
				"universe: snapshot await timed out",
				dialCtx.Err(),
			))
		default:
		}

		msgType, payload, err := conn.ReadMessage()
		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.IO,
				"universe: read message from websocket",
				err,
			))
		}

		if msgType != websocket.TextMessage || len(payload) == 0 {
			continue
		}

		var snapshot InstrumentSnapshotMessage
		if err := json.Unmarshal(payload, &snapshot); err != nil {
			continue
		}

		if snapshot.Channel != "instrument" || snapshot.Type != "snapshot" {
			continue
		}

		symbols := make([]string, 0, len(snapshot.Data.Pairs))
		for _, pair := range snapshot.Data.Pairs {
			if strings.ToUpper(pair.Quote) != normalizedQuote {
				continue
			}

			if pair.Status != "online" {
				continue
			}

			baseUpper := strings.ToUpper(strings.TrimSpace(pair.Base))
			if _, excluded := excludedSet[baseUpper]; excluded {
				continue
			}

			symbols = append(symbols, pair.Symbol)
		}

		slices.Sort(symbols)
		return symbols, nil
	}
}
