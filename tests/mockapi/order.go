package mockapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/theapemachine/errnie"
	orderackfixture "github.com/theapemachine/symm/tests/fixtures/orderack"
)

/*
paperOrder is one validated market or resting-limit instruction.
*/
type paperOrder struct {
	id           string
	reqID        int64
	symbol       string
	side         string
	typ          string
	quantity     float64
	limit        float64
	maker        bool
	reserveAsset string
	reserve      float64
}

/*
orderRequest preserves exact Kraken order numbers during request decoding.
*/
type orderRequest struct {
	Method string `json:"method"`
	ReqID  int64  `json:"req_id"`
	Params struct {
		OrderType  string      `json:"order_type"`
		Side       string      `json:"side"`
		OrderQty   json.Number `json:"order_qty"`
		Symbol     string      `json:"symbol"`
		LimitPrice json.Number `json:"limit_price"`
	} `json:"params"`
}

/*
Handle validates one add_order request and queues its acknowledgement plus any
immediate crossing fill.
*/
func (paper *Paper) Handle(raw []byte) ([]outbound, error) {
	request, err := decodeOrder(raw)

	if err != nil {
		return nil, err
	}

	paper.mu.Lock()
	defer paper.mu.Unlock()
	paper.nextID++
	order := paperOrder{
		id:       fmt.Sprintf("PAPER-%05d", paper.nextID),
		reqID:    request.ReqID,
		symbol:   request.Params.Symbol,
		side:     request.Params.Side,
		typ:      request.Params.OrderType,
		quantity: number(request.Params.OrderQty),
		limit:    number(request.Params.LimitPrice),
	}
	bid, _, ask, _, _ := paper.options.Quote(order.symbol)
	order.maker = order.typ == "limit" &&
		!(order.side == "buy" && order.limit >= ask ||
			order.side == "sell" && order.limit <= bid)

	if err := paper.validate(order); err != nil {
		return nil, err
	}

	frames := []outbound{{
		channel: "add_order",
		payload: orderackfixture.Frame(orderackfixture.Options{
			ReqID:   order.reqID,
			OrderID: order.id,
			Success: true,
		}),
	}}

	if !order.maker {
		return append(frames, paper.fill(order, bid, ask)...), nil
	}

	order = paper.reserve(order)
	paper.open[order.id] = order
	paper.order = append(paper.order, order.id)
	frames = append(frames, paper.execution(order, 0, "new", "open"))
	return frames, nil
}

/*
validate rejects orders that cannot be executed or funded by the paper ledger.
*/
func (paper *Paper) validate(order paperOrder) error {
	if order.symbol == "" || order.quantity <= 0 ||
		order.side != "buy" && order.side != "sell" ||
		order.typ != "market" && order.typ != "limit" {
		return errnie.Err(errnie.Validation, "tests/mockapi: valid order required", nil)
	}

	if order.typ == "limit" && order.limit <= 0 {
		return errnie.Err(errnie.Validation, "tests/mockapi: limit price required", nil)
	}

	bid, bidQty, ask, askQty, exists := paper.options.Quote(order.symbol)

	if !exists || bid <= 0 || ask <= bid {
		return errnie.Err(errnie.Validation, "tests/mockapi: executable quote required", nil)
	}

	if !order.maker && (order.side == "buy" && order.quantity > askQty ||
		order.side == "sell" && order.quantity > bidQty) {
		return errnie.Err(errnie.Validation, "tests/mockapi: order exceeds touch liquidity", nil)
	}

	base, quote, ok := strings.Cut(order.symbol, "/")

	if !ok {
		return errnie.Err(errnie.Validation, "tests/mockapi: normalized order symbol required", nil)
	}

	price := order.limit

	if !order.maker && order.side == "buy" {
		price = ask
	}

	if !order.maker && order.side == "sell" {
		price = bid
	}

	if order.side == "buy" && paper.available(quote) < order.quantity*price*(1+paper.fee(order)) ||
		order.side == "sell" && paper.available(base) < order.quantity {
		return errnie.Err(errnie.Validation, "tests/mockapi: insufficient paper balance", nil)
	}

	return nil
}

/*
decodeOrder preserves exact JSON numbers from an add_order request.
*/
func decodeOrder(raw []byte) (orderRequest, error) {
	request := orderRequest{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	if err := decoder.Decode(&request); err != nil {
		return request, errnie.Err(errnie.Validation, "tests/mockapi: decode order", err)
	}

	return request, nil
}

/*
number parses an optional exact wire number.
*/
func number(value json.Number) float64 {
	parsed, _ := value.Float64()
	return parsed
}
