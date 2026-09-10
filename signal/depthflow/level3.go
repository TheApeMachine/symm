package depthflow

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

// Level3 owns per-symbol chronology; its graphs consume only facts from the
// current mutation message. No untouched book orders are carried forward.
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
	fields, err := transport.Evaluate(graph, transport.Values(DepthInput{
		ObservedBid: observedBid, ObservedAsk: observedAsk, AddBid: addBid, AddAsk: addAsk,
		ModifyBid: modifyBid, ModifyAsk: modifyAsk, DeleteBid: deleteBid, DeleteAsk: deleteAsk,
		MutationBid: float64(len(message.Bids)), MutationAsk: float64(len(message.Asks)), Elapsed: elapsed,
	}))
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}
	level3.lastTime[message.Symbol] = message.Timestamp
	level3.projection.Identity = func() (string, string, time.Time, time.Time) {
		return message.Symbol + ":depthflow:" + message.Timestamp.Format(time.RFC3339Nano), message.Symbol, message.Timestamp, message.Timestamp
	}
	return level3.projection.Project(fields)
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
