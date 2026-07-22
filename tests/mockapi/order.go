package mockapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
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
	quantity, err := number(request.Params.OrderQty)

	if err != nil {
		return nil, errnie.Err(errnie.Validation, "tests/mockapi: invalid order quantity", err)
	}

	limit := 0.0

	if request.Params.LimitPrice != "" {
		limit, err = number(request.Params.LimitPrice)

		if err != nil {
			return nil, errnie.Err(errnie.Validation, "tests/mockapi: invalid limit price", err)
		}
	}

	order := paperOrder{
		reqID:    request.ReqID,
		symbol:   request.Params.Symbol,
		side:     request.Params.Side,
		typ:      request.Params.OrderType,
		quantity: quantity,
		limit:    limit,
	}
	bid, bidQty, ask, askQty, exists := paper.options.Quote(order.symbol)
	order.maker = order.typ == "limit" &&
		!(order.side == "buy" && order.limit >= ask ||
			order.side == "sell" && order.limit <= bid)

	if err := paper.validate(
		order,
		bid,
		bidQty,
		ask,
		askQty,
		exists,
	); err != nil {
		return nil, err
	}

	paper.nextID++
	order.id = fmt.Sprintf("PAPER-%05d", paper.nextID)

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
	zero := decimal.NewFromInt64(0).SetScale(8)
	frames = append(frames, paper.execution(order, 0, zero, zero, "new", "open"))
	return frames, nil
}

/*
validate rejects orders that cannot be executed or funded by the paper ledger.
*/
func (paper *Paper) validate(
	order paperOrder,
	bid float64,
	bidQty float64,
	ask float64,
	askQty float64,
	exists bool,
) error {
	if order.symbol == "" || order.quantity <= 0 ||
		order.side != "buy" && order.side != "sell" ||
		order.typ != "market" && order.typ != "limit" {
		return errnie.Err(errnie.Validation, "tests/mockapi: valid order required", nil)
	}

	if order.typ == "limit" && order.limit <= 0 {
		return errnie.Err(errnie.Validation, "tests/mockapi: limit price required", nil)
	}

	if !exists || bid <= 0 || ask <= bid ||
		math.IsInf(bid, 0) || math.IsInf(ask, 0) ||
		math.IsNaN(bid) || math.IsNaN(ask) {
		return errnie.Err(errnie.Validation, "tests/mockapi: executable quote required", nil)
	}

	for _, value := range []float64{bid, bidQty, ask, askQty} {
		if _, err := number(json.Number(strconv.FormatFloat(value, 'g', -1, 64))); err != nil {
			return errnie.Err(errnie.Validation, "tests/mockapi: eight-decimal quote required", err)
		}
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

	quantity, err := decimal.NewFromString(strconv.FormatFloat(
		order.quantity, 'f', 8, 64,
	))

	if err != nil {
		return errnie.Err(errnie.Validation, "tests/mockapi: exact order quantity required", err)
	}

	required := quantity
	available := paper.available(base)

	if order.side == "buy" {
		executionPrice, err := decimal.NewFromString(strconv.FormatFloat(
			price, 'f', 8, 64,
		))

		if err != nil {
			return errnie.Err(errnie.Validation, "tests/mockapi: exact order price required", err)
		}

		feeRate, err := decimal.NewFromString(strconv.FormatFloat(
			paper.fee(order), 'f', 8, 64,
		))

		if err != nil {
			return errnie.Err(errnie.Validation, "tests/mockapi: exact fee required", err)
		}

		cost := decimal.ExactMul(quantity, executionPrice).SetScale(8)
		required = cost.Add(decimal.ExactMul(cost.Copy(), feeRate).SetScale(8))
		available = paper.available(quote)
	}

	if available.Cmp(required) < 0 {
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

/* number parses one wire number that the eight-decimal ledger can preserve. */
func number(value json.Number) (float64, error) {
	exact, err := decimal.NewFromString(value.String())

	if err != nil {
		return 0, errnie.Err(errnie.Validation, "tests/mockapi: finite number required", err)
	}

	parsed := exact.Float64()
	quantized := exact.Copy().SetScale(8)
	maxExact := math.Ldexp(1, 53) / math.Pow10(8)

	if math.IsInf(parsed, 0) || math.IsNaN(parsed) ||
		exact.Cmp(quantized) != 0 || math.Abs(parsed) > maxExact {
		return 0, errnie.Err(
			errnie.Validation,
			"tests/mockapi: finite eight-decimal number required",
			nil,
		)
	}

	return parsed, nil
}
