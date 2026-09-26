package kraken

import (
	"context"
	"encoding/json"
	"github.com/theapemachine/errnie"
	"strings"
)

/* UniverseServer owns the quote and base-asset eligibility decision at venue ingress. */
type UniverseServer struct{ symbols []string }

/* NewUniverse constructs an idle instrument reader. */
func NewUniverse() *UniverseServer { return &UniverseServer{} }

/* Write selects every online pair matching the declared quote and exclusion policy. */
func (server *UniverseServer) Write(ctx context.Context, call Universe_write) error {
	server.symbols = nil
	payload, err := call.Args().Data()

	if err != nil {
		return errnie.Error(err)
	}

	if len(payload) == 0 {
		return nil
	}
	quote, err := call.Args().Quote()

	if err != nil {
		return errnie.Error(err)
	}

	if quote == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "universe: quote is required", nil))
	}
	excluded, err := call.Args().Excluded()

	if err != nil {
		return errnie.Error(err)
	}
	blocked := make(map[string]bool, excluded.Len())
	for index := range excluded.Len() {
		asset, err := excluded.At(index)

		if err != nil {
			return errnie.Error(err)
		}
		blocked[strings.ToUpper(asset)] = true
	}
	var frame struct {
		Channel string          `json:"channel"`
		Data    json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(payload, &frame); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "universe: invalid venue frame", err))
	}

	if frame.Channel != "instrument" {
		return nil
	}
	var instruments struct {
		Pairs []struct{ Symbol, Base, Quote, Status string } `json:"pairs"`
	}

	if err := json.Unmarshal(frame.Data, &instruments); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "universe: invalid instruments", err))
	}
	seen := make(map[string]bool)
	for _, pair := range instruments.Pairs {
		if pair.Symbol == "" || pair.Base == "" || pair.Quote == "" || pair.Status == "" {
			return errnie.Error(errnie.Err(errnie.Validation, "universe: incomplete instrument pair", nil))
		}

		if pair.Quote != quote || pair.Status != "online" || blocked[strings.ToUpper(pair.Base)] || seen[pair.Symbol] {
			continue
		}
		seen[pair.Symbol] = true
		server.symbols = append(server.symbols, pair.Symbol)
	}
	return nil
}

/* Done publishes native membership and the two external Kraken subscription messages. */
func (server *UniverseServer) Done(ctx context.Context, call Universe_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}

	if len(server.symbols) == 0 {
		result.SetIdle()
		return nil
	}
	defer func() { server.symbols = nil }()
	result.SetReady()
	symbols, err := result.Ready().NewSymbols(int32(len(server.symbols)))

	if err != nil {
		return errnie.Error(err)
	}
	for index, symbol := range server.symbols {
		if err := symbols.Set(index, symbol); err != nil {
			return errnie.Error(err)
		}
	}
	for _, channel := range []string{"ticker", "trade"} {
		message := struct {
			Method string `json:"method"`
			Params struct {
				Channel      string   `json:"channel"`
				Symbol       []string `json:"symbol"`
				Snapshot     bool     `json:"snapshot"`
				EventTrigger string   `json:"event_trigger,omitempty"`
			} `json:"params"`
		}{Method: "subscribe"}
		message.Params.Channel, message.Params.Symbol = channel, server.symbols

		if channel == "ticker" {
			message.Params.Snapshot = true
			message.Params.EventTrigger = "bbo"
		}
		encoded, err := json.Marshal(message)

		if err != nil {
			return errnie.Error(err)
		}

		if channel == "ticker" {
			err = result.Ready().SetTicker(encoded)
		}

		if channel == "trade" {
			err = result.Ready().SetTrade(encoded)
		}

		if err != nil {
			return errnie.Error(err)
		}
	}
	return nil
}
