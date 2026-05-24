package trader

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/client"
	"github.com/theapemachine/symm/kraken/order"
)

const liveOrderTimeout = 30 * time.Second

/*
KrakenBroker submits live spot orders over Kraken WebSocket v2.
*/
type KrakenBroker struct {
	client    *client.PrivateClient
	pairIndex map[string]asset.Pair
	feePct    float64
}

/*
NewKrakenBroker wires a private websocket client for live order placement.
*/
func NewKrakenBroker(
	privateClient *client.PrivateClient,
	pairIndex map[string]asset.Pair,
	feePct float64,
) *KrakenBroker {
	return &KrakenBroker{
		client:    privateClient,
		pairIndex: pairIndex,
		feePct:    feePct,
	}
}

/*
Live reports true for Kraken order placement.
*/
func (krakenBroker *KrakenBroker) Live() bool {
	return true
}

/*
Enter places a market order and waits for the execution fill.
*/
func (krakenBroker *KrakenBroker) Enter(
	ctx context.Context,
	request BrokerEnterRequest,
) (BrokerFill, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, liveOrderTimeout)
	defer cancel()

	if request.Side == positionShort {
		return krakenBroker.enterShort(timeoutCtx, request)
	}

	return krakenBroker.enterLong(timeoutCtx, request)
}

func (krakenBroker *KrakenBroker) enterLong(
	ctx context.Context,
	request BrokerEnterRequest,
) (BrokerFill, error) {
	limitBelow := request.StopPrice * 0.999

	if limitBelow <= 0 {
		limitBelow = request.StopPrice
	}

	frame := order.MarketBuyCash(
		request.Symbol,
		request.NotionalEUR,
		request.StopPrice,
		limitBelow,
		krakenBroker.client.TokenValue(),
	)

	ack, err := krakenBroker.client.PlaceOrder(ctx, frame)

	if err != nil {
		return BrokerFill{}, err
	}

	fill, err := krakenBroker.client.WaitFill(ctx, ack.Result.OrderID)

	if err != nil {
		return BrokerFill{}, err
	}

	proceeds := fill.Qty * fill.Price
	fee := config.System.TakerFee(proceeds, krakenBroker.feePct)

	return BrokerFill{
		FillPrice: fill.Price,
		BaseQty:   fill.Qty,
		FeeEUR:    fee,
		OrderID:   ack.Result.OrderID,
	}, nil
}

func (krakenBroker *KrakenBroker) enterShort(
	ctx context.Context,
	request BrokerEnterRequest,
) (BrokerFill, error) {
	baseQty := roundBaseQty(
		request.NotionalEUR/request.Last,
		krakenBroker.lotDecimals(request.Symbol),
	)

	if baseQty <= 0 {
		return BrokerFill{}, fmt.Errorf("invalid short base qty for %s", request.Symbol)
	}

	frame := order.MarketSellBase(request.Symbol, baseQty, krakenBroker.client.TokenValue())

	ack, err := krakenBroker.client.PlaceOrder(ctx, frame)

	if err != nil {
		return BrokerFill{}, err
	}

	fill, err := krakenBroker.client.WaitFill(ctx, ack.Result.OrderID)

	if err != nil {
		return BrokerFill{}, err
	}

	proceeds := fill.Qty * fill.Price
	fee := config.System.TakerFee(proceeds, krakenBroker.feePct)

	return BrokerFill{
		FillPrice: fill.Price,
		BaseQty:   fill.Qty,
		FeeEUR:    fee,
		OrderID:   ack.Result.OrderID,
	}, nil
}

/*
Exit closes an open position with a market order.
*/
func (krakenBroker *KrakenBroker) Exit(
	ctx context.Context,
	request BrokerExitRequest,
) (BrokerFill, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, liveOrderTimeout)
	defer cancel()

	baseQty := request.BaseQty

	if baseQty <= 0 && request.Last > 0 {
		baseQty = request.NotionalEUR / request.Last
	}

	baseQty = roundBaseQty(baseQty, krakenBroker.lotDecimals(request.Symbol))

	if baseQty <= 0 {
		return BrokerFill{}, fmt.Errorf("invalid exit base qty for %s", request.Symbol)
	}

	var frame order.Request

	if request.Side == positionShort {
		frame = order.MarketBuyBase(request.Symbol, baseQty, krakenBroker.client.TokenValue())
	}

	if request.Side != positionShort {
		frame = order.MarketSellBase(request.Symbol, baseQty, krakenBroker.client.TokenValue())
	}

	ack, err := krakenBroker.client.PlaceOrder(timeoutCtx, frame)

	if err != nil {
		return BrokerFill{}, err
	}

	fill, err := krakenBroker.client.WaitFill(timeoutCtx, ack.Result.OrderID)

	if err != nil {
		return BrokerFill{}, err
	}

	proceeds := fill.Qty * fill.Price
	fee := config.System.TakerFee(proceeds, krakenBroker.feePct)

	return BrokerFill{
		FillPrice: fill.Price,
		BaseQty:   fill.Qty,
		FeeEUR:    fee,
		OrderID:   ack.Result.OrderID,
	}, nil
}

/*
AmendStop ratchets a resting exchange stop trigger price.
*/
func (krakenBroker *KrakenBroker) AmendStop(
	ctx context.Context,
	request BrokerAmendStopRequest,
) error {
	if request.OrderID == "" {
		return fmt.Errorf("stop order id is required")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, liveOrderTimeout)
	defer cancel()

	frame := order.AmendStopTrigger(
		request.OrderID,
		request.TriggerPrice,
		krakenBroker.client.TokenValue(),
	)

	_, err := krakenBroker.client.AmendOrder(timeoutCtx, frame)

	return err
}

func (krakenBroker *KrakenBroker) lotDecimals(symbol string) int {
	pair, ok := krakenBroker.pairIndex[symbol]

	if !ok || pair.LotDecimals <= 0 {
		return 8
	}

	return pair.LotDecimals
}

func roundBaseQty(qty float64, lotDecimals int) float64 {
	if qty <= 0 {
		return 0
	}

	scale := math.Pow10(lotDecimals)

	return math.Floor(qty*scale) / scale
}
