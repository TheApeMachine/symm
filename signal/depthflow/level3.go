package depthflow

import (
	"fmt"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
	"sync"
	"time"
)

// Level3 owns per-symbol chronology; its graphs consume only facts from the
// current mutation message. No untouched book orders are carried forward.
type Level3 struct {
	mu         sync.Mutex
	graphs     map[string]core.Primitive
	lastTime   map[string]time.Time
	projection *data.Projection
}

func NewLevel3() *Level3 {
	return &Level3{graphs: make(map[string]core.Primitive), lastTime: make(map[string]time.Time), projection: depthProjection()}
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
	level3.mu.Lock()
	defer level3.mu.Unlock()
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
	fields, err := transport.Evaluate[map[string]core.Primitive](graph, core.Record(map[string]any{
		"observed_notional:bid": observedBid, "observed_notional:ask": observedAsk,
		"add_notional:bid": addBid, "add_notional:ask": addAsk,
		"modify_remaining_notional:bid": modifyBid, "modify_remaining_notional:ask": modifyAsk,
		"delete_count:bid": deleteBid, "delete_count:ask": deleteAsk,
		"mutation_count:bid": float64(len(message.Bids)), "mutation_count:ask": float64(len(message.Asks)), "elapsed": elapsed,
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
