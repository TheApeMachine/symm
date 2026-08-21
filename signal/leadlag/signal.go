package leadlag

import (
	"context"
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	thesis *types.Thesis
	number *nomagique.Number[string]
	pair   nomagique.Primitive
	work   *transport.Consumer[*types.Symbol]
}

func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		number: nomagique.NewNumber[string](temporal.Path),
		pair:   algo.LeadLag(),
	}
	signal.work = transport.NewConsumer[*types.Symbol](signal.Name(), signal.consume)
	thesis.Work(types.SourceLeadLag).Register(signal.work)

	return signal
}

func (signal *Signal) Name() string           { return string(types.SourceLeadLag) }
func (signal *Signal) Error() error           { return signal.err }
func (signal *Signal) Type() types.SourceType { return types.SourceLeadLag }

func (signal *Signal) consume() {
	go func() {
		defer func() {
			signal.thesis.Fail(signal.err)
		}()

		for symbol := range signal.thesis.Work(types.SourceLeadLag).Drain(signal.work, nil) {
			select {
			case <-signal.ctx.Done():
				signal.err = signal.ctx.Err()
				return
			default:
			}

			if symbol == nil {
				continue
			}

			for ticker := range symbol.MarketTickers(
				symbol.TickerConsumers[types.TickerConsumerLeadLag],
			) {
				price, observed, err := tickerPrice(ticker)

				if err != nil {
					signal.err = errnie.Error(errnie.Err(
						errnie.Validation,
						"leadlag: invalid ticker for "+symbol.Symbol,
						err,
					))

					return
				}

				if !observed {
					continue
				}

				anchor, _, _, hasAnchor, err := signal.number.ArgMax(
					correlation.Return,
					correlation.SymbolMagnitude,
					correlation.SymbolReady,
				)

				if err != nil {
					signal.err = errnie.Error(errnie.Err(
						errnie.Validation,
						"leadlag: anchor selection failed",
						err,
					))

					return
				}

				input := nomagique.Frame{}
				input.Put(nomagique.SampleValue, price)
				input.Put(nmtypes.EventTimeSec, float64(ticker.Timestamp.Unix()))
				input.Put(nmtypes.EventTimeNsec, float64(ticker.Timestamp.Nanosecond()))
				_, err = signal.number.Step(symbol.Symbol, input)

				if err != nil {
					signal.err = errnie.Error(errnie.Err(
						errnie.Validation,
						"leadlag: path failed for "+symbol.Symbol,
						err,
					))

					return
				}

				if !hasAnchor || anchor == symbol.Symbol {
					symbol.AppendMeasurement(signal.neutralMeasurement(
						symbol.Symbol, ticker.Timestamp, price,
					))

					continue
				}

				anchorPath, hasAnchorPath := signal.number.Project(anchor)
				followerPath, hasFollowerPath := signal.number.Project(symbol.Symbol)

				if !hasAnchorPath || !hasFollowerPath {
					symbol.AppendMeasurement(signal.neutralMeasurement(
						symbol.Symbol, ticker.Timestamp, price,
					))

					continue
				}

				_, output, err := signal.pair(anchorPath, followerPath)

				if err != nil {
					signal.err = errnie.Error(errnie.Err(
						errnie.Validation,
						"leadlag: pair failed for "+symbol.Symbol,
						err,
					))

					return
				}

				ready, _ := output.Get(equation.SymbolLeadLagReady)

				symbol.AppendMeasurement(signal.measurement(
					symbol.Symbol,
					anchor,
					ticker.Timestamp,
					anchorPath,
					followerPath,
					output,
					ready != 0,
				))
			}
		}
	}()
}

func tickerPrice(ticker kraken.TickerData) (float64, bool, error) {
	if ticker.Last == nil {
		return 0, false, fmt.Errorf("leadlag: ticker requires a last price")
	}

	if ticker.Last.Sign() < 0 {
		return 0, false, fmt.Errorf("leadlag: ticker last price cannot be negative")
	}

	if ticker.Last.Sign() == 0 {
		return 0, false, nil
	}

	return ticker.Last.Float64(), true, nil
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
