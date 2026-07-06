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
	OpenOrders []PaperOrder `json:"open_orders"`
	Mode       string       `json:"mode"`
}

type PaperTrade struct {
	Cost    float64 `json:"cost"`
	Fee     float64 `json:"fee"`
	ID      string  `json:"id"`
	OrderID string  `json:"order_id"`
	Pair    string  `json:"pair"`
	Price   float64 `json:"price"`
	Side    string  `json:"side"`
	Status  string  `json:"status"`
	Time    string  `json:"time"`
	Volume  float64 `json:"volume"`
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
			Balance:   balance.Total,
			Available: balance.Available,
			Reserved:  balance.Reserved,
		})
	}

	return rows, nil
}

func (paper *PaperCLI) Orders(ctx context.Context) ([]PaperOrder, error) {
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

		rows = append(rows, ExecutionData{
			AvgPrice:    trade.Price,
			Cost:        trade.Cost,
			Fee:         trade.Fee,
			ExecID:      trade.ID,
			LastPrice:   trade.Price,
			LastQty:     trade.Volume,
			OrderID:     trade.OrderID,
			OrderQty:    trade.Volume,
			OrderStatus: trade.Status,
			Side:        trade.Side,
			Symbol:      trade.Pair,
			Timestamp:   stamp,
		})
	}

	return rows, nil
}

func (paper *PaperCLI) Submit(ctx context.Context, order *Order) error {
	if order == nil {
		return errnie.Error(errnie.Err(
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

	return errnie.Error(errnie.Err(
		errnie.Validation,
		"kraken paper unsupported method: "+order.Method,
		nil,
	))
}

func (paper *PaperCLI) add(ctx context.Context, order *Order) error {
	params := LimitOrderParams{}
	raw, err := sonic.Marshal(order.Params)

	if err != nil {
		return err
	}

	if err := sonic.Unmarshal(raw, &params); err != nil {
		return err
	}

	side := strings.TrimSpace(params.Side)
	symbol := strings.ReplaceAll(strings.TrimSpace(params.Symbol), "/", "")
	quantity := strconv.FormatFloat(params.OrderQty, 'f', -1, 64)

	if side != "buy" && side != "sell" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper side must be buy or sell",
			nil,
		))
	}

	if symbol == "" || params.OrderQty <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper order requires symbol and positive quantity",
			nil,
		))
	}

	args := []string{"paper", side, "-o", "json"}

	if strings.ToLower(strings.TrimSpace(params.OrderType)) == "limit" {
		if params.LimitPrice <= 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"kraken paper limit order requires positive price",
				nil,
			))
		}

		args = append(args, "--type", "limit", "--price")
		args = append(args, strconv.FormatFloat(params.LimitPrice, 'f', -1, 64))
	}

	args = append(args, symbol, quantity)
	_, err = paper.run(ctx, args...)
	return err
}

func (paper *PaperCLI) cancel(ctx context.Context, order *Order) error {
	params := map[string]any{}
	raw, err := sonic.Marshal(order.Params)

	if err != nil {
		return err
	}

	if err := sonic.Unmarshal(raw, &params); err != nil {
		return err
	}

	orderID, _ := params["order_id"].(string)
	orderID = strings.TrimSpace(orderID)

	if orderID == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"kraken paper cancel requires order_id",
			nil,
		))
	}

	_, err = paper.run(ctx, "paper", "cancel", "-o", "json", orderID)
	return err
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
