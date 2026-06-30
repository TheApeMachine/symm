package manifold

import (
	"encoding/json"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

func (signal *Signal) hydrateFieldFromTree() {
	if signal == nil || signal.field == nil || signal.tree == nil {
		return
	}

	prev := signal.lastHydrateStamp
	now := time.Now().UTC()

	floor := prev
	if floor <= 1 {
		floor = now.Add(-1 * time.Hour).UnixNano()
	}

	latest := floor

	var symbols []string
	signal.field.universe.states.Range(func(key, value any) bool {
		if state, ok := value.(*UniverseState); ok {
			symbols = append(symbols, state.symbol)
		}
		return true
	})

	scopes := make([]string, 0, len(symbols)+1)
	scopes = append(scopes, "update")
	scopes = append(scopes, symbols...)

	for _, role := range []string{"book", "trade", "ticker"} {
		var start time.Time
		if prev <= 1 {
			start = now.Add(-1 * time.Hour).UTC().Truncate(24 * time.Hour)
		} else {
			start = time.Unix(0, prev).UTC().Truncate(24 * time.Hour)
		}
		end := now.UTC().Truncate(24 * time.Hour)
		if end.Before(start) {
			end = start
		}

		for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
			dayStr := cursor.Format("2006/01/02")
			seekKeys := [][]byte{[]byte(role + "/" + dayStr + "/")}

			for _, scope := range scopes {
				seekKeys = append(seekKeys, []byte(role+"/"+scope+"/"+dayStr+"/"))
				seekKeys = append(seekKeys, []byte(role+"/"+scope+"/kraken/"+dayStr+"/"))
			}

			for _, seekKey := range seekKeys {
				for inbound := range signal.tree.Seek(seekKey) {
					stamp := inbound.Timestamp()

					if stamp > 0 && stamp <= floor {
						continue
					}

					if stamp > now.UnixNano() {
						break
					}

					r, err := inbound.Role()
					if err != nil || r != role {
						break
					}

					switch role {
					case "book":
						signal.observeBookArtifact(inbound)
					case "trade":
						signal.observeTradeArtifact(inbound)
					case "ticker":
						signal.observeTickerArtifact(inbound)
					}

					if stamp > latest {
						latest = stamp
					}
				}
			}
		}
	}

	if latest > signal.lastHydrateStamp {
		signal.lastHydrateStamp = latest
	}
}

func treeArtifactPayload(artifact *datura.Artifact) []byte {
	if artifact == nil || !artifact.HasPayload() {
		return nil
	}

	return artifact.DecryptPayload()
}

func forEachKrakenElement(payload []byte, visit func(element []byte)) {
	if len(payload) == 0 || visit == nil {
		return
	}

	var envelope struct {
		Data []json.RawMessage `json:"data"`
	}

	if json.Unmarshal(payload, &envelope) == nil && len(envelope.Data) > 0 {
		for _, row := range envelope.Data {
			if len(row) > 0 {
				visit(row)
			}
		}

		return
	}

	visit(payload)
}

func krakenEnvelopeType(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}

	var envelope struct {
		Type string `json:"type"`
	}

	if json.Unmarshal(payload, &envelope) != nil {
		return ""
	}

	return envelope.Type
}

func (signal *Signal) observeBookArtifact(artifact *datura.Artifact) {
	payload := treeArtifactPayload(artifact)

	if len(payload) == 0 {
		return
	}

	envelopeType := krakenEnvelopeType(payload)

	forEachKrakenElement(payload, func(element []byte) {
		var update BookUpdate

		if json.Unmarshal(element, &update) == nil && update.Symbol != "" {
			if update.Type == "" {
				update.Type = envelopeType
			}

			signal.observeBookUpdate(update)
		}
	})
}

func (signal *Signal) observeBookUpdate(update BookUpdate) {
	if update.Symbol == "" {
		return
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now().UTC()
	}

	signal.field.SetInstrumentTick(update.Symbol, signal.instrumentTick(update.Symbol))
	if err := signal.field.FeedBook(update, eventAt); err != nil {
		panic(errnie.Err(errnie.Validation, "manifold: book feed failed for "+update.Symbol+": "+err.Error(), err))
	}
}

/*
instrumentTick reads the exchange tick_size for symbol from the tree catalog the
discover path populates. Returns 0 when the pair is not yet catalogued, leaving
the field to infer the tick from book gaps until the instrument frame arrives.
*/
func (signal *Signal) instrumentTick(symbol string) float64 {
	if signal.tree == nil {
		return 0
	}

	raw, ok := signal.tree.Get([]byte("instrument/" + symbol + "/"))

	if !ok {
		return 0
	}

	artifact := datura.Acquire("manifold", datura.APPJSON)
	defer artifact.Release()

	if _, err := artifact.Unpack(raw); err != nil {
		return 0
	}

	var meta struct {
		TickSize float64 `json:"tick_size"`
	}

	if json.Unmarshal(artifact.DecryptPayload(), &meta) != nil {
		return 0
	}

	return meta.TickSize
}

func (signal *Signal) observeTradeArtifact(artifact *datura.Artifact) {
	payload := treeArtifactPayload(artifact)

	if len(payload) == 0 {
		return
	}

	forEachKrakenElement(payload, func(element []byte) {
		var update TradeUpdate

		if json.Unmarshal(element, &update) == nil && update.Symbol != "" {
			signal.observeTradeUpdate(update)
		}
	})
}

func (signal *Signal) observeTradeUpdate(update TradeUpdate) {
	if update.Symbol == "" || update.Price <= 0 {
		return
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now().UTC()
	}

	row := update
	if err := signal.field.FeedTrade(&row, eventAt); err != nil {
		panic(errnie.Err(errnie.Validation, "manifold: trade feed failed for "+update.Symbol+": "+err.Error(), err))
	}
}

func (signal *Signal) observeTickerArtifact(artifact *datura.Artifact) {
	payload := treeArtifactPayload(artifact)

	if len(payload) == 0 {
		return
	}

	forEachKrakenElement(payload, func(element []byte) {
		var update TickerUpdate

		if json.Unmarshal(element, &update) == nil && update.Symbol != "" {
			signal.observeTickerUpdate(update)
		}
	})
}

func (signal *Signal) observeTickerUpdate(update TickerUpdate) {
	if update.Symbol == "" {
		return
	}

	price := update.Last

	if price <= 0 && update.Bid > 0 && update.Ask > update.Bid {
		price = (update.Bid + update.Ask) / 2
	}

	if price <= 0 {
		return
	}

	eventAt := update.Timestamp

	if eventAt.IsZero() {
		eventAt = time.Now().UTC()
	}

	row := update
	row.Last = price
	if err := signal.field.FeedTicker(row, eventAt); err != nil {
		panic(errnie.Err(errnie.Validation, "manifold: ticker feed failed for "+update.Symbol+": "+err.Error(), err))
	}
}
