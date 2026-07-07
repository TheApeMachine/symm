package kraken

import (
	"bytes"
	"context"
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

func (paper *PaperCLI) Balances(ctx context.Context) ([]BalanceData, error) {
	buf, err := paper.run(ctx, "paper", "balance", "-o", "json")

	if err != nil {
		return nil, err
	}

	response := PaperBalanceResponse{}

	if err := sonic.Unmarshal(buf, &response); err != nil {
		return nil, errnie.Error(errnie.Err(
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
			Asset:     asset,
			Balance:   *decimal.NewFromFloat64(balance.Total),
			Available: *decimal.NewFromFloat64(balance.Available),
			Reserved:  *decimal.NewFromFloat64(balance.Reserved),
		})
	}

	return rows, nil
}

func (paper *PaperCLI) Orders(ctx context.Context) ([]OrderData, error) {
	buf, err := paper.run(ctx, "paper", "orders", "-o", "json")

	if err != nil {
		return nil, err
	}

	response := PaperOrdersResponse{}

	if err := sonic.Unmarshal(buf, &response); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"kraken paper orders: invalid json",
			err,
		))
	}

	for index := range response.OpenOrders {
		response.OpenOrders[index].Pair = paper.symbol(response.OpenOrders[index].Pair)
		response.OpenOrders[index].Description.Pair = response.OpenOrders[index].Pair
	}

	return response.OpenOrders, nil
}

func (paper *PaperCLI) Executions(ctx context.Context) ([]ExecutionData, error) {
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
