package depthflow

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the depth-flow measuring instrument. It composes its market entity
in its constructor and exposes the canonical signal structure: Constructor,
Name, Error, Step, Close.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	level3 *Level3
}

/*
NewSignal composes the Level3 (depth-flow) entity.
*/
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		level3: NewLevel3(),
	}
}

func (signal *Signal) Name() string { return "depthflow" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	if signal.err != nil {
		errnie.Error(signal.Close())
		return nil
	}

	/*
		A signal observes exactly the envelope kind it consumes. Stepping on any
		other kind hands the estimator a zero-valued observation, which it
		correctly rejects — and that rejection becomes a Measurement carrying an
		Err. data.Lift discards the WHOLE frame on the first failed measurement,
		so one signal stepped out of turn erased every other signal's metrics
		from the same envelope, and no advisor could ever assemble a complete
		feature group.
	*/
	if envelope.TypeID != types.EnvelopeLevel3 {
		return envelope
	}

	envelope.DepthFlow = signal.level3.Step(envelope.Level3Data)

	return envelope
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return signal.level3.Close()
}

type Level3 struct {
	graphs     map[string]*Depth
	lastTime   map[string]time.Time
	projection *data.Projection
}

func NewLevel3() *Level3 {
	return &Level3{graphs: make(map[string]*Depth), lastTime: make(map[string]time.Time), projection: depthProjection()}
}

func (level3 *Level3) Step(message kraken.Level3Data) *data.Measurement[float64] {
	if level3 == nil || len(message.Bids)+len(message.Asks) == 0 {
		return nil
	}
	observedBid, addBid, modifyBid, deleteBid, err := observeSide(message.Bids)
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	observedAsk, addAsk, modifyAsk, deleteAsk, err := observeSide(message.Asks)
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}

	last, hasLast := level3.lastTime[message.Symbol]
	if hasLast && message.Timestamp.Before(last) {
		return nil
	}
	elapsed := 0.0
	if hasLast {
		elapsed = message.Timestamp.Sub(last).Seconds()
	}
	graph := level3.graphs[message.Symbol]
	if graph == nil {
		graph = newDepthGraph()
		level3.graphs[message.Symbol] = graph
	}
	fieldsEval := transport.NewEvaluate(graph)
	var fields data.ProjectionInput

	for out := range fieldsEval.Next(transport.NewValues(DepthInput{
		ObservedBid: observedBid, ObservedAsk: observedAsk, AddBid: addBid, AddAsk: addAsk,
		ModifyBid: modifyBid, ModifyAsk: modifyAsk, DeleteBid: deleteBid, DeleteAsk: deleteAsk,
		MutationBid: float64(len(message.Bids)), MutationAsk: float64(len(message.Asks)), Elapsed: elapsed,
	}).Next(nil)) {
		fields = *(*data.ProjectionInput)(out)
	}

	err = fieldsEval.Error()
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	level3.lastTime[message.Symbol] = message.Timestamp
	level3.projection.Identity = func() (string, string, time.Time, time.Time) {
		return message.Symbol + ":depthflow:" + message.Timestamp.Format(time.RFC3339Nano), message.Symbol, message.Timestamp, message.Timestamp
	}
	resultEval := transport.NewEvaluate(level3.projection)
	var result *data.Measurement[float64]

	for out := range resultEval.Next(transport.NewValues(fields).Next(nil)) {
		result = *(**data.Measurement[float64])(out)
	}

	err = resultEval.Error()

	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}

	return result
}

func observeSide(orders []kraken.Level3Order) (observed, added, modified, deleted float64, err error) {
	for _, order := range orders {
		if order.LimitPrice == nil || order.OrderQty == nil {
			return 0, 0, 0, 0, fmt.Errorf("depthflow: level3 order requires price and quantity")
		}

		price := order.LimitPrice.Float64()
		quantity := order.OrderQty.Float64()

		if price <= 0 || quantity < 0 {
			return 0, 0, 0, 0, fmt.Errorf("depthflow: level3 order requires positive price and non-negative quantity")
		}

		notional := price * quantity
		observed += notional

		switch order.Event {
		case "", "add":
			added += notional
		case "modify":
			modified += notional
		case "delete":
			deleted++
		default:
			return 0, 0, 0, 0, fmt.Errorf("depthflow: unknown level3 event %q", order.Event)
		}
	}

	return observed, added, modified, deleted, nil
}

/*
Close releases resources held by the Level3 entity.
*/
func (level3 *Level3) Close() error {
	return nil
}
