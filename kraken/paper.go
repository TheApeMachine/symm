package kraken

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

type PaperCLI struct {
	Command string
}

type PaperBalance struct {
	Available float64 `json:"available"`
	Reserved  float64 `json:"reserved"`
	Total     float64 `json:"total"`
}

type PaperBalanceResponse struct {
	Balances map[string]PaperBalance `json:"balances"`
	Mode     string                  `json:"mode"`
}

type PaperHistoryResponse struct {
	Trades []PaperTrade `json:"trades"`
	Mode   string       `json:"mode"`
}

type PaperOrdersResponse struct {
	OpenOrders []OrderData `json:"open_orders"`
	Mode       string      `json:"mode"`
}

type PaperTrade struct {
	Cost    decimal.Decimal `json:"cost"`
	Fee     decimal.Decimal `json:"fee"`
	ID      string          `json:"id"`
	OrderID string          `json:"order_id"`
	Pair    string          `json:"pair"`
	Price   decimal.Decimal `json:"price"`
	Side    string          `json:"side"`
	Status  string          `json:"status"`
	Time    string          `json:"time"`
	Volume  decimal.Decimal `json:"volume"`
}

type PaperSubmitResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func NewPaperCLI() *PaperCLI {
	return &PaperCLI{Command: "kraken"}
}

func (paper *PaperCLI) Balances(ctx context.Context) (Balance, error) {
	buf, err := paper.run(ctx, "paper", "balance", "-o", "json")

	if err != nil {
		return Balance{}, err
	}

	response := PaperBalanceResponse{}

	if err := sonic.Unmarshal(buf, &response); err != nil {
		return Balance{}, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"kraken paper balance: invalid json",
			err,
		))
	}

	assets := make([]string, 0, len(response.Balances))

	for asset := range response.Balances {
		assets = append(assets, asset)
	}

	sort.Strings(assets)
	rows := make([]BalanceData, 0, len(assets))

	for _, asset := range assets {
		balance := response.Balances[asset]
		rows = append(rows, BalanceData{
			Asset:      asset,
			AssetClass: "currency",
			Balance:    *decimal.NewFromFloat64(balance.Total),
			Available:  *decimal.NewFromFloat64(balance.Available),
			Reserved:   *decimal.NewFromFloat64(balance.Reserved),
		})
	}

	return Balance{
		Channel:  "balances",
		Type:     "snapshot",
		Data:     rows,
		Sequence: 1,
	}, nil
}

func (paper *PaperCLI) Orders(ctx context.Context) (Orders, error) {
	buf, err := paper.run(ctx, "paper", "orders", "-o", "json")

	if err != nil {
		return Orders{}, err
	}

	response := PaperOrdersResponse{}

	if err := sonic.Unmarshal(buf, &response); err != nil {
		return Orders{}, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"kraken paper orders: invalid json",
			err,
		))
	}

	for index := range response.OpenOrders {
		response.OpenOrders[index].Pair = paper.symbol(response.OpenOrders[index].Pair)
		response.OpenOrders[index].Description.Pair = response.OpenOrders[index].Pair
	}

	return Orders{
		Channel:  "orders",
		Type:     "snapshot",
		Data:     response.OpenOrders,
		Sequence: 1,
	}, nil
}

func (paper *PaperCLI) Executions(ctx context.Context) (Execution, error) {
	openRows, err := paper.openPositions(ctx)

	if err != nil {
		return Execution{}, err
	}

	historyRows, err := paper.executionHistory(ctx)

	if err != nil {
		return Execution{}, err
	}

	return Execution{
		Channel:  "executions",
		Type:     "snapshot",
		Data:     append(openRows, historyRows...),
		Sequence: 1,
	}, nil
}

func (paper *PaperCLI) executionHistory(ctx context.Context) ([]ExecutionData, error) {
	buf, err := paper.run(ctx, "paper", "history", "-o", "json")

	if err != nil {
		return nil, err
	}

	response := PaperHistoryResponse{}

	if err := sonic.Unmarshal(buf, &response); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"kraken paper history: invalid json",
			err,
		))
	}

	rows := make([]ExecutionData, 0, len(response.Trades))

	for _, trade := range response.Trades {
		stamp, err := time.Parse(time.RFC3339Nano, trade.Time)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"kraken paper history: invalid trade time",
				err,
			))
		}

		if trade.Price.Rat().Sign() <= 0 ||
			trade.Cost.Rat().Sign() <= 0 ||
			trade.Volume.Rat().Sign() <= 0 ||
			trade.Fee.Rat().Sign() < 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken paper history: invalid trade economics",
				nil,
			))
		}

		qty, _ := trade.Volume.Rat().Float64()

		rows = append(rows, ExecutionData{
			AvgPrice:    trade.Price,
			Cost:        trade.Cost,
			ExecID:      trade.ID,
			ExecType:    "history",
			FeeUSDEquiv: trade.Fee,
			LastPrice:   trade.Price,
			LastQty:     qty,
			OrderID:     trade.OrderID,
			OrderQty:    qty,
			OrderStatus: trade.Status,
			Side:        trade.Side,
			Symbol:      paper.symbol(trade.Pair),
			Timestamp:   stamp,
		})
	}

	return rows, nil
}

/*
openPositions folds paper trade history into Kraken-style open-position
snapshot rows for desk hydration on startup.
*/
func (paper *PaperCLI) openPositions(ctx context.Context) ([]ExecutionData, error) {
	buf, err := paper.run(ctx, "paper", "history", "-o", "json")

	if err != nil {
		return nil, err
	}

	response := PaperHistoryResponse{}

	if err := sonic.Unmarshal(buf, &response); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"kraken paper history: invalid json",
			err,
		))
	}

	type foldedPosition struct {
		qty   *big.Rat
		cost  *big.Rat
		stamp time.Time
		side  string
	}

	positions := map[string]*foldedPosition{}

	for _, trade := range response.Trades {
		symbol := paper.symbol(trade.Pair)
		stamp, err := time.Parse(time.RFC3339Nano, trade.Time)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"kraken paper history: invalid trade time",
				err,
			))
		}

		if trade.Price.Rat().Sign() <= 0 ||
			trade.Cost.Rat().Sign() <= 0 ||
			trade.Volume.Rat().Sign() <= 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken paper history: invalid trade economics",
				nil,
			))
		}

		fold := positions[symbol]

		if fold == nil {
			fold = &foldedPosition{
				qty:  new(big.Rat),
				cost: new(big.Rat),
			}
			positions[symbol] = fold
		}

		volume := trade.Volume.Rat()
		cost := trade.Cost.Rat()
		side := strings.ToLower(strings.TrimSpace(trade.Side))

		switch side {
		case "buy":
			fold.qty.Add(fold.qty, volume)
			fold.cost.Add(fold.cost, cost)
			fold.stamp = stamp
			fold.side = side
		case "sell":
			if fold.qty.Sign() <= 0 {
				continue
			}

			sold := new(big.Rat).Set(volume)

			if sold.Cmp(fold.qty) > 0 {
				sold.Set(fold.qty)
			}

			costSold := new(big.Rat).Mul(
				fold.cost,
				new(big.Rat).Quo(sold, fold.qty),
			)
			fold.qty.Sub(fold.qty, sold)
			fold.cost.Sub(fold.cost, costSold)
		default:
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken paper history: unsupported side "+trade.Side,
				nil,
			))
		}
	}

	symbols := make([]string, 0, len(positions))

	for symbol, fold := range positions {
		if fold.qty.Sign() > 0 {
			symbols = append(symbols, symbol)
		}
	}

	sort.Strings(symbols)
	rows := make([]ExecutionData, 0, len(symbols))

	for _, symbol := range symbols {
		fold := positions[symbol]
		qty, _ := fold.qty.Float64()
		avgPrice := new(big.Rat).Quo(fold.cost, fold.qty)
		avgDecimal, err := decimal.NewFromString(avgPrice.FloatString(16))

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken paper open positions: invalid average price",
				err,
			))
		}

		costDecimal, err := decimal.NewFromString(fold.cost.FloatString(16))

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken paper open positions: invalid cost",
				err,
			))
		}

		rows = append(rows, ExecutionData{
			AvgPrice:       *avgDecimal,
			Cost:           *costDecimal,
			ExecID:         "paper-open-" + strings.ReplaceAll(symbol, "/", ""),
			ExecType:       "snapshot",
			LastPrice:      *avgDecimal,
			LastQty:        qty,
			CumQty:         qty,
			OrderQty:       qty,
			OrderStatus:    "filled",
			PositionStatus: "open",
			Side:           fold.side,
			Symbol:         symbol,
			Timestamp:      fold.stamp,
		})
	}

	return rows, nil
}

func (paper *PaperCLI) Submit(
	ctx context.Context,
	order *Order,
) (*OrderResponse, error) {
	if order == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper order required",
			nil,
		))
	}

	switch strings.TrimSpace(order.Method) {
	case "add_order":
		return paper.add(ctx, order)
	case "cancel_order":
		return paper.cancel(ctx, order)
	}

	return nil, errnie.Error(errnie.Err(
		errnie.Validation,
		"kraken paper unsupported method: "+order.Method,
		nil,
	))
}

func (paper *PaperCLI) add(
	ctx context.Context,
	order *Order,
) (*OrderResponse, error) {
	params := LimitOrderParams{}
	raw, err := sonic.Marshal(order.Params)

	if err != nil {
		return nil, err
	}

	if err := sonic.Unmarshal(raw, &params); err != nil {
		return nil, err
	}

	side := strings.TrimSpace(params.Side)
	symbol := strings.ReplaceAll(strings.TrimSpace(params.Symbol), "/", "")
	quantity := strconv.FormatFloat(params.OrderQty, 'f', -1, 64)

	if side != "buy" && side != "sell" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper side must be buy or sell",
			nil,
		))
	}

	if symbol == "" || params.OrderQty <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper order requires symbol and positive quantity",
			nil,
		))
	}

	args := []string{"paper", side, "-o", "json"}

	if strings.ToLower(strings.TrimSpace(params.OrderType)) == "limit" {
		if params.LimitPrice <= 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken paper limit order requires positive price",
				nil,
			))
		}

		args = append(args, "--type", "limit", "--price")
		args = append(args, strconv.FormatFloat(params.LimitPrice, 'f', -1, 64))
	}

	args = append(args, symbol, quantity)
	buf, err := paper.run(ctx, args...)

	if err != nil {
		return nil, err
	}

	response := PaperSubmitResponse{}

	if err := sonic.Unmarshal(buf, &response); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"kraken paper add_order: invalid json",
			err,
		))
	}

	return &OrderResponse{
		Method:  "add_order",
		ReqID:   order.ReqID,
		Success: true,
		Result: OrderResponseResult{
			OrderID: response.ID,
		},
	}, nil
}

func (paper *PaperCLI) cancel(
	ctx context.Context,
	order *Order,
) (*OrderResponse, error) {
	params := map[string]any{}
	raw, err := sonic.Marshal(order.Params)

	if err != nil {
		return nil, err
	}

	if err := sonic.Unmarshal(raw, &params); err != nil {
		return nil, err
	}

	orderID, _ := params["order_id"].(string)
	orderID = strings.TrimSpace(orderID)

	if orderID == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper cancel requires order_id",
			nil,
		))
	}

	if _, err := paper.run(ctx, "paper", "cancel", "-o", "json", orderID); err != nil {
		return nil, err
	}

	return &OrderResponse{
		Method:  "cancel_order",
		ReqID:   order.ReqID,
		Success: true,
		Result: OrderResponseResult{
			OrderID: orderID,
		},
	}, nil
}

/*
Post serves paper-backed REST endpoints through the Kraken CLI adapter.
*/
func (paper *PaperCLI) Post(
	ctx context.Context,
	path string,
	params json.Marshaler,
) ([]byte, error) {
	switch strings.TrimSpace(path) {
	case TradeVolumeEndpoint:
		return paper.tradeVolume(params)
	default:
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper post: unsupported path "+path,
			nil,
		))
	}
}

func (paper *PaperCLI) tradeVolume(params json.Marshaler) ([]byte, error) {
	raw, err := params.MarshalJSON()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper trade volume marshal failed",
			err,
		))
	}

	request := TradeVolumeRequest{}

	if err := sonic.Unmarshal(raw, &request); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper trade volume decode failed",
			err,
		))
	}

	takerRate := viper.GetFloat64("trading.paper.taker_fee_bps") / 10000
	makerRate := viper.GetFloat64("trading.paper.maker_fee_bps") / 10000

	if takerRate <= 0 || makerRate <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper trade volume: trading.paper fee bps required",
			nil,
		))
	}

	rates := FeeRates{
		Taker: takerRate,
		Maker: makerRate,
	}
	pairs := make(map[string]FeeRates, len(request.Pairs))

	for _, symbol := range request.Pairs {
		symbol = strings.TrimSpace(symbol)

		if symbol == "" {
			continue
		}

		pairs[symbol] = rates
	}

	return sonic.Marshal(FeeSchedule{
		Pairs: pairs,
	})
}

func (paper *PaperCLI) symbol(pair string) string {
	pair = strings.TrimSpace(pair)

	if strings.Contains(pair, "/") {
		return pair
	}

	quote := strings.ToUpper(strings.TrimSpace(
		viper.GetString("market.quote_currency"),
	))

	if quote == "" || !strings.HasSuffix(pair, quote) {
		return pair
	}

	return strings.TrimSuffix(pair, quote) + "/" + quote
}

func (paper *PaperCLI) run(ctx context.Context, args ...string) ([]byte, error) {
	commandName := strings.TrimSpace(paper.Command)

	if commandName == "" {
		commandName = "kraken"
	}

	command := exec.CommandContext(ctx, commandName, args...)
	stderr := bytes.Buffer{}
	command.Stderr = &stderr
	buf, err := command.Output()

	if err != nil {
		message := strings.TrimSpace(stderr.String())

		if message == "" {
			message = err.Error()
		}

		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"kraken paper cli: "+message,
			err,
		))
	}

	return buf, nil
}
