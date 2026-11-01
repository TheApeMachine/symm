package strategy

import (
	"context"
	"fmt"
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

/*
Tick is the last LMAX stage: persist the full envelope, publish a light frame.
*/
type Tick struct {
	*runtime.System
	store *store.Client
}

/*
Light is the UI-sized tick: paths, decisions, and tape symbols — no signal maps.
*/
type Light struct {
	Epoch     uint64                       `json:"epoch"`
	N         uint64                       `json:"n"`
	Symbols   []string                     `json:"symbols"`
	Paths     map[string]string            `json:"paths"`
	Decisions map[string]*runtime.Decision `json:"decisions"`
}

func NewTick(ctx context.Context) *Tick {
	return &Tick{
		System: runtime.NewSystem(ctx, "tick"),
		store:  store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
	}
}

func (tick *Tick) Step(envelope *runtime.Envelope) *runtime.Envelope {
	if envelope == nil {
		return nil
	}

	tick.storeFull(envelope)
	tick.publishLight(envelope)

	return envelope
}

func (tick *Tick) storeFull(envelope *runtime.Envelope) {
	if envelope == nil {
		return
	}

	buf, err := runtime.Sonic.Marshal(envelope)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[tick] marshal envelope", err,
		))

		return
	}

	epoch := data.Epoch.Load()
	n := data.Tick.Load()
	key := fmt.Sprintf(
		"workspace/tick/aggregate/%d/%019d",
		epoch, n,
	)

	if _, err := tick.store.WriteAtPrefix(key, buf); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[tick] store envelope "+key, err,
		))
	}
}

func (tick *Tick) publishLight(envelope *runtime.Envelope) {
	if envelope == nil || ui.UI == nil || !ui.UI.Active() {
		return
	}

	light := tick.light(envelope)

	frame, err := runtime.Sonic.Marshal(map[string]any{"tick": light})

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[tick] marshal light frame", err,
		))

		return
	}

	ui.UI.Write(frame)

	if len(light.Decisions) == 0 {
		return
	}

	if decisions := envelope.Publish("decisions"); len(decisions) > 0 {
		ui.UI.Write(decisions)
	}
}

func (tick *Tick) light(envelope *runtime.Envelope) Light {
	paths := make(map[string]string, len(envelope.Paths))
	symbols := make([]string, 0, len(envelope.Paths))

	for symbol, cut := range envelope.Paths {
		if symbol == "" {
			continue
		}

		symbols = append(symbols, symbol)

		if cut != nil {
			paths[symbol] = cut.String()
		}
	}

	if envelope.Trade != nil {
		for _, row := range envelope.Trade.Data {
			if row == nil || row.Symbol == "" {
				continue
			}

			symbols = append(symbols, row.Symbol)
		}
	}

	return Light{
		Epoch:     data.Epoch.Load(),
		N:         data.Tick.Load(),
		Symbols:   compactSymbols(symbols),
		Paths:     paths,
		Decisions: envelope.Decisions,
	}
}

func compactSymbols(symbols []string) []string {
	slices.Sort(symbols)

	return slices.Compact(symbols)
}
