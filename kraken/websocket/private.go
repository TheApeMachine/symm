package websocket

import (
	"context"
	"math"
	"math/big"
	"os"
	"sort"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	kfx "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

type Private interface {
	Observe(string) chan []byte
	Submit(*kraken.Order) error
	TradeVolume(pairs []string) (float64, error)
	Close()
}

func NewPrivate(ctx context.Context) Private {
	if viper.GetString("trading.model") == "live" {
		return NewLivePrivate(ctx)
	}

	return NewPaperPrivate(ctx)
}

type LivePrivate struct {
	ctx        context.Context
	cancel     context.CancelFunc
	client     *spot.WebSocket
	url        string
	publicKey  string
	privateKey string
	stream     *Stream
}

type tradeVolumeResult struct {
	Fees map[string]tradeVolumeFee `json:"fees"`
}

type tradeVolumeFee struct {
	Fee decimal.Decimal `json:"fee"`
}

func NewLivePrivate(ctx context.Context) *LivePrivate {
	ctx, cancel := context.WithCancel(ctx)
	buffer := viper.GetViper().GetInt("system.websocket.channel.buffer")

	private := &LivePrivate{
		ctx:        ctx,
		cancel:     cancel,
		client:     spot.NewWebSocket(),
		url:        "ws-auth.kraken.com/v2",
		publicKey:  os.Getenv("KRAKEN_API_KEY"),
		privateKey: os.Getenv("KRAKEN_API_SECRET"),
		stream:     NewStream(buffer),
	}

	private.configure()

	private.client.OnSent.Recurring(func(e *callback.Event[*kfx.WebSocketMessage]) {
		private.checkContext()
	})

	private.client.OnReceived.Recurring(func(e *callback.Event[*kfx.WebSocketMessage]) {
		private.checkContext()
		private.stream.Receive(e.Data.Bytes())
	})

	private.client.OnAuthenticated.Recurring(func(e *callback.Event[string]) {
		private.checkContext()
		errnie.Error(private.client.SubBalances())
		errnie.Error(private.client.SubExecutions())
	})

	private.client.OnConnected.Recurring(func(e *callback.Event[any]) {
		private.checkContext()
		errnie.Error(private.client.Authenticate())
	})

	errnie.Error(private.client.Connect())

	return private
}

func (private *LivePrivate) Observe(channel string) chan []byte {
	observer := private.stream.Observe(channel)

	switch channel {
	case "balances":
		errnie.Error(private.publishBalances())
	case "executions":
		errnie.Error(private.publishExecutions())
	case "orders":
		errnie.Error(private.publishOrders())
	}

	return observer
}

func (private *LivePrivate) Submit(order *kraken.Order) error {
	if order == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken: private order required",
			nil,
		))
	}

	params := map[string]any{}
	raw, err := sonic.Marshal(order.Params)
	if err != nil {
		return err
	}

	if err := sonic.Unmarshal(raw, &params); err != nil {
		return err
	}

	switch strings.TrimSpace(order.Method) {
	case "add_order":
		orderType, _ := params["order_type"].(string)
		side, _ := params["side"].(string)
		quantity, _ := params["order_qty"].(float64)
		symbol, _ := params["symbol"].(string)

		if strings.TrimSpace(orderType) == "" ||
			strings.TrimSpace(side) == "" ||
			strings.TrimSpace(symbol) == "" ||
			quantity <= 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken: live add_order requires order_type, side, symbol, and positive order_qty",
				nil,
			))
		}

		return private.client.AddOrder(
			strings.TrimSpace(orderType),
			strings.TrimSpace(side),
			quantity,
			strings.TrimSpace(symbol),
			map[string]any{"params": params, "req_id": order.ReqID},
		)
	case "cancel_order":
		return private.client.CancelOrder(map[string]any{
			"params": params,
			"req_id": order.ReqID,
		})
	}

	return errnie.Error(errnie.Err(
		errnie.Validation,
		"kraken: unsupported private method: "+order.Method,
		nil,
	))
}

func (private *LivePrivate) Close() {
	private.cancel()
	private.client.Disconnect()
}

func (private *LivePrivate) TradeVolume(pairs []string) (float64, error) {
	symbols := make([]string, 0, len(pairs))

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)

		if pair != "" {
			symbols = append(symbols, pair)
		}
	}

	body := map[string]any{
		"fee-info": true,
	}

	if len(symbols) > 0 {
		body["pair"] = strings.Join(symbols, ",")
	}

	response, err := spot.Call[tradeVolumeResult](private.client.REST, spot.RequestOptions{
		Auth:   true,
		Method: "POST",
		Path:   "/0/private/TradeVolume",
		Body:   body,
	})

	if err != nil {
		return 0, errnie.Error(err)
	}

	return response.Result.TakerRate()
}

func (result tradeVolumeResult) TakerRate() (float64, error) {
	var maxFee *decimal.Decimal

	for _, row := range result.Fees {
		if row.Fee.Rat().Sign() < 0 {
			return 0, errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken: trade volume fee must be non-negative",
				nil,
			))
		}

		fee := row.Fee

		if maxFee == nil || fee.Rat().Cmp(maxFee.Rat()) > 0 {
			maxFee = &fee
		}
	}

	if maxFee == nil || maxFee.Rat().Sign() <= 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken: trade volume response missing taker fees",
			nil,
		))
	}

	rate := maxFee.Float64() / 100

	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken: invalid trade volume fee rate",
			nil,
		))
	}

	return rate, nil
}

func (private *LivePrivate) publishBalances() error {
	response, err := private.client.REST.Balances()

	if err != nil {
		return err
	}

	rows := kraken.NewBalanceDataSliceFromSpot(response.Result)
	return private.publish("balances", rows)
}

func (private *LivePrivate) publishExecutions() error {
	response, err := private.client.REST.TradesHistory(&spot.TradesHistoryRequest{
		Trades: true,
	})

	if err != nil {
		return err
	}

	type openTrade struct {
		qty        *big.Rat
		cost       *big.Rat
		fee        *big.Rat
		priceScale int
		costScale  int
		feeScale   int
		row        kraken.ExecutionData
	}

	positions := map[string]*openTrade{}
	trades := make([]struct {
		id    string
		trade spot.Trade
	}, 0, len(response.Result.Trades))

	for id, trade := range response.Result.Trades {
		symbol := strings.TrimSpace(trade.Pair)

		if symbol == "" ||
			!strings.Contains(symbol, "/") ||
			trade.Time == nil ||
			trade.Volume == nil ||
			trade.Price == nil ||
			trade.Cost == nil ||
			trade.Fee == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken: trade history row missing broker execution fields",
				nil,
			))
		}

		if trade.Volume.Rat().Sign() <= 0 ||
			trade.Price.Rat().Sign() <= 0 ||
			trade.Cost.Rat().Sign() <= 0 ||
			trade.Fee.Rat().Sign() < 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken: trade history row has invalid economics",
				nil,
			))
		}

		side := strings.ToLower(strings.TrimSpace(trade.Type))

		if side != "buy" && side != "sell" {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken: trade history row has unsupported side",
				nil,
			))
		}

		trades = append(trades, struct {
			id    string
			trade spot.Trade
		}{id: id, trade: trade})
	}

	sort.Slice(trades, func(left int, right int) bool {
		return trades[left].trade.Time.Rat().Cmp(trades[right].trade.Time.Rat()) < 0
	})

	for _, history := range trades {
		trade := history.trade
		symbol := strings.TrimSpace(trade.Pair)
		side := strings.ToLower(strings.TrimSpace(trade.Type))

		position := positions[symbol]

		if position == nil {
			position = &openTrade{
				qty:  new(big.Rat),
				cost: new(big.Rat),
				fee:  new(big.Rat),
			}
			positions[symbol] = position
		}

		if scale := int(trade.Price.GetScale()); scale > position.priceScale {
			position.priceScale = scale
		}

		if scale := int(trade.Cost.GetScale()); scale > position.costScale {
			position.costScale = scale
		}

		if scale := int(trade.Fee.GetScale()); scale > position.feeScale {
			position.feeScale = scale
		}

		row := kraken.ExecutionData{
			AvgPrice:    *trade.Price,
			Cost:        *trade.Cost,
			ExecID:      history.id,
			FeeUSDEquiv: *trade.Fee,
			LastPrice:   *trade.Price,
			OrderID:     trade.OrderID,
			OrderStatus: "filled",
			OrderType:   trade.OrderType,
			Side:        trade.Type,
			Symbol:      symbol,
		}

		switch side {
		case "buy":
			position.qty.Add(position.qty, trade.Volume.Rat())
			position.cost.Add(position.cost, trade.Cost.Rat())
			position.fee.Add(position.fee, trade.Fee.Rat())
			position.row = row
		case "sell":
			if position.qty.Sign() <= 0 || trade.Volume.Rat().Cmp(position.qty) > 0 {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"kraken: trade history sell exceeds restored position",
					nil,
				))
			}

			ratio := new(big.Rat).Quo(trade.Volume.Rat(), position.qty)
			costReduction := new(big.Rat).Mul(new(big.Rat).Set(position.cost), ratio)
			feeReduction := new(big.Rat).Mul(new(big.Rat).Set(position.fee), ratio)
			position.cost.Sub(position.cost, costReduction)
			position.fee.Sub(position.fee, feeReduction)
			position.qty.Sub(position.qty, trade.Volume.Rat())
		}
	}

	rows := kraken.ExecutionDataSlice{}

	for _, position := range positions {
		if position.qty.Sign() <= 0 || position.cost.Sign() <= 0 {
			continue
		}

		price, err := decimal.NewFromString(
			new(big.Rat).Quo(position.cost, position.qty).FloatString(position.priceScale),
		)

		if err != nil {
			return errnie.Error(err)
		}

		cost, err := decimal.NewFromString(position.cost.FloatString(position.costScale))

		if err != nil {
			return errnie.Error(err)
		}

		fee, err := decimal.NewFromString(position.fee.FloatString(position.feeScale))

		if err != nil {
			return errnie.Error(err)
		}

		qty, _ := position.qty.Float64()
		row := position.row
		row.AvgPrice = *price
		row.Cost = *cost
		row.ExecType = "snapshot"
		row.FeeUSDEquiv = *fee
		row.LastPrice = row.AvgPrice
		row.LastQty = qty
		row.OrderQty = qty
		row.OrderStatus = "filled"
		row.PositionStatus = "open"
		row.Side = "buy"
		rows = append(rows, row)
	}

	return private.publish("executions", rows)
}

func (private *LivePrivate) publishOrders() error {
	response, err := private.client.REST.OpenOrders(&spot.OpenOrdersRequest{
		Trades: true,
	})

	if err != nil {
		return err
	}

	rows := kraken.NewOrderDataSliceFromSpot(response.Result.Open)
	return private.publish("orders", rows)
}

func (private *LivePrivate) publish(channel string, rows any) error {
	buf, err := sonic.Marshal(rows)

	if err != nil {
		return err
	}

	private.stream.Receive(append(
		[]byte(`{"channel":"`+channel+`","data":`),
		append(buf, '}')...,
	))

	return nil
}

func (private *LivePrivate) configure() {
	if strings.TrimSpace(private.url) != "" {
		private.client.URL = private.url
	}

	if private.publicKey == "" {
		private.publicKey = os.Getenv("SYMM_KRAKEN_API_KEY")
	}

	if private.privateKey == "" {
		private.privateKey = os.Getenv("SYMM_KRAKEN_API_SECRET")
	}

	private.client.REST.PublicKey = private.publicKey
	private.client.REST.PrivateKey = private.privateKey
}

func (private *LivePrivate) checkContext() {
	select {
	case <-private.ctx.Done():
		private.Close()
	default:
	}
}
