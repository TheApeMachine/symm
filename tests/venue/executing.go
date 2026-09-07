package venue

import (
	"encoding/json"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"sync"
)

/*
Executing is a mock transport whose AddOrder returns a synthetic order result
and — like the paper simulator — pushes a filled execution envelope into the
execution Workload so the same delivery path the live wiring uses advances the
position. It captures Submitted order requests so tests can assert the client
order ID survived the trip.
*/
type Executing struct {
	*Conn

	Workload *nmruntime.Workload[*types.Envelope]

	Mu        sync.Mutex
	Submitted []*spot.AddOrderRequest
}

func (conn *Executing) MarkReady() {}

func NewExecuting(Workload *nmruntime.Workload[*types.Envelope]) *Executing {
	return &Executing{
		Conn:     NewConn(),
		Workload: Workload,
	}
}

/*
Deliver pushes one filled execution record for the Submitted order into the
execution Workload, exactly as the paper transport publishes a fill.
*/
func (conn *Executing) Deliver(execution kraken.ExecutionData) {
	envelope := types.NewEnvelope(types.EnvelopeExecution)
	envelope.ExecutionData = execution
	conn.Workload.Push(envelope)
}

func (conn *Executing) AddOrder(order *spot.AddOrderRequest) (spot.AddOrderResult, error) {
	conn.Mu.Lock()
	conn.Submitted = append(conn.Submitted, order)
	conn.Mu.Unlock()

	return spot.AddOrderResult{
		OrderPlacementSingle: spot.OrderPlacementSingle{ID: []string{"venue-order-1"}},
	}, nil
}

func (conn *Executing) SubInstrument(callback chan any) {
	// Seed one tradeable pair so the desk can resolve its instrument.
	callback <- &kraken.Instrument{Data: kraken.InstrumentData{
		Pairs: []kraken.InstrumentPair{
			{Symbol: "TEST/USD", Base: "TEST", Quote: "USD", Status: "online", TickSize: *decimal.NewFromFloat64(0.01)},
		},
	}}
}

func (conn *Executing) Write(json.Marshaler, ...websocket.Callback[any]) error { return nil }
