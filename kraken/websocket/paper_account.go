package websocket

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

type CommandRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

type KrakenCLI struct {
	binary string
}

func NewKrakenCLI(binary string) *KrakenCLI {
	if strings.TrimSpace(binary) == "" {
		binary = "kraken"
	}

	return &KrakenCLI{binary: binary}
}

func (runner *KrakenCLI) Run(ctx context.Context, args ...string) ([]byte, error) {
	if runner == nil {
		runner = NewKrakenCLI("")
	}

	argv := append([]string(nil), args...)
	argv = append(argv, "-o", "json")

	cmd := exec.CommandContext(ctx, runner.binary, argv...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}

	detail := strings.TrimSpace(stderr.String())
	if detail == "" {
		detail = err.Error()
	}

	return nil, fmt.Errorf("kraken %s: %s: %w", strings.Join(argv, " "), detail, err)
}

type PaperAccount struct {
	ctx        context.Context
	cancel     context.CancelFunc
	runner     CommandRunner
	observers  []chan map[string]any
	buffer     int
	seenTrades map[string]struct{}
	synced     bool
}

func NewPaperAccount(ctx context.Context) *PaperAccount {
	return NewPaperAccountWithRunner(ctx, NewKrakenCLI(""))
}

func NewPaperAccountWithRunner(ctx context.Context, runner CommandRunner) *PaperAccount {
	ctx, cancel := context.WithCancel(ctx)
	if runner == nil {
		runner = NewKrakenCLI("")
	}

	return &PaperAccount{
		ctx:        ctx,
		cancel:     cancel,
		runner:     runner,
		buffer:     viper.GetViper().GetInt("system.websocket.channel.buffer"),
		seenTrades: map[string]struct{}{},
	}
}

func (account *PaperAccount) Observe() chan map[string]any {
	out := make(chan map[string]any, account.buffer)
	account.observers = append(account.observers, out)
	return out
}

func (account *PaperAccount) Submit(artifact *datura.Artifact) error {
	request, err := NewOrderRequest(artifact)
	if err != nil {
		return err
	}

	switch request.Method {
	case "add_order":
		return account.addOrder(request)
	case "cancel_order":
		return account.cancelOrder(request)
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"paper account: unsupported private request method: "+request.Method,
			nil,
		))
	}
}

func (account *PaperAccount) Sync() error {
	balances, err := account.paperBalances()
	if err != nil {
		return err
	}

	orders, err := account.paperOrders()
	if err != nil {
		return err
	}

	history, err := account.paperHistory()
	if err != nil {
		return err
	}

	account.emit(account.balanceFrame(balances))
	account.emit(account.ordersFrame(orders))
	for _, frame := range account.executionFrames(history) {
		account.emit(frame)
	}

	account.synced = true
	return nil
}

func (account *PaperAccount) Close() {
	account.cancel()
}

func (account *PaperAccount) addOrder(request *OrderRequest) error {
	quantity, err := request.Float("order_qty")
	if err != nil {
		return err
	}

	side := request.String("side")
	if side != "buy" && side != "sell" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"paper account: order side must be buy or sell",
			nil,
		))
	}

	orderType := request.String("order_type")
	if orderType != "market" && orderType != "limit" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"paper account: unsupported order type: "+orderType,
			nil,
		))
	}

	args := []string{
		"paper",
		side,
		"--type",
		orderType,
		paperPair(request.String("symbol")),
		strconv.FormatFloat(quantity, 'f', -1, 64),
	}

	if orderType == "limit" {
		price, err := request.Float("limit_price")
		if err != nil {
			return err
		}

		args = append(args[:4], append(
			[]string{"--price", strconv.FormatFloat(price, 'f', -1, 64)},
			args[4:]...,
		)...)
	}

	if _, err := account.runner.Run(account.ctx, args...); err != nil {
		return err
	}

	return account.Sync()
}

func (account *PaperAccount) cancelOrder(request *OrderRequest) error {
	orderID := request.String("cl_ord_id")
	if orderID == "" {
		orderID = request.String("order_id")
	}

	if orderID == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"paper account: cancel order id required",
			nil,
		))
	}

	if _, err := account.runner.Run(account.ctx, "paper", "cancel", orderID); err != nil {
		return err
	}

	return account.Sync()
}

func (account *PaperAccount) paperBalances() (*paperBalanceResponse, error) {
	raw, err := account.runner.Run(account.ctx, "paper", "balance")
	if err != nil {
		return nil, err
	}

	var out paperBalanceResponse
	if err := sonic.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("kraken paper balance: decode json: %w", err)
	}

	return &out, nil
}

func (account *PaperAccount) paperOrders() (*paperOrdersResponse, error) {
	raw, err := account.runner.Run(account.ctx, "paper", "orders")
	if err != nil {
		return nil, err
	}

	var out paperOrdersResponse
	if err := sonic.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("kraken paper orders: decode json: %w", err)
	}

	return &out, nil
}

func (account *PaperAccount) paperHistory() (*paperHistoryResponse, error) {
	raw, err := account.runner.Run(account.ctx, "paper", "history")
	if err != nil {
		return nil, err
	}

	var out paperHistoryResponse
	if err := sonic.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("kraken paper history: decode json: %w", err)
	}

	return &out, nil
}

func (account *PaperAccount) balanceFrame(response *paperBalanceResponse) map[string]any {
	if response == nil || len(response.Balances) == 0 {
		return nil
	}

	assets := make([]string, 0, len(response.Balances))
	for asset := range response.Balances {
		assets = append(assets, asset)
	}
	sort.Strings(assets)

	data := make([]map[string]any, 0, len(assets))
	for _, asset := range assets {
		balance := response.Balances[asset]
		data = append(data, map[string]any{
			"asset":       strings.ToUpper(asset),
			"asset_class": "currency",
			"available":   balance["available"],
			"reserved":    balance["reserved"],
			"balance":     balance["total"],
			"wallets": []map[string]any{{
				"balance": balance["total"],
				"type":    "spot",
				"id":      "paper",
			}},
		})
	}

	return map[string]any{
		"channel":   "balances",
		"type":      account.frameType(),
		"data":      data,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (account *PaperAccount) ordersFrame(response *paperOrdersResponse) map[string]any {
	if response == nil {
		return nil
	}

	for _, order := range response.OpenOrders {
		pair := stringField(order, "pair")
		if pair == "" {
			pair = stringField(order, "symbol")
		}
		if pair != "" {
			order["symbol"] = normalizePair(pair)
		}
		if _, ok := order["status"]; !ok {
			order["status"] = "open"
		}
	}

	return map[string]any{
		"channel":   "orders",
		"type":      account.frameType(),
		"data":      response.OpenOrders,
		"count":     len(response.OpenOrders),
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (account *PaperAccount) executionFrames(response *paperHistoryResponse) []map[string]any {
	if response == nil || len(response.Trades) == 0 {
		return nil
	}

	out := make([]map[string]any, 0)
	for _, trade := range response.Trades {
		id := stringField(trade, "id")
		if id == "" {
			id = stringField(trade, "order_id")
		}
		if id == "" {
			continue
		}

		if _, ok := account.seenTrades[id]; ok {
			continue
		}

		account.seenTrades[id] = struct{}{}
		if !account.synced {
			continue
		}

		pair := stringField(trade, "pair")
		timestamp := stringField(trade, "time")
		if timestamp == "" {
			timestamp = time.Now().UTC().Format(time.RFC3339Nano)
		}

		out = append(out, map[string]any{
			"channel": "executions",
			"type":    "update",
			"data": []map[string]any{{
				"exec_id":      id,
				"order_id":     stringField(trade, "order_id"),
				"symbol":       normalizePair(pair),
				"side":         strings.ToLower(stringField(trade, "side")),
				"order_status": "filled",
				"order_qty":    trade["volume"],
				"last_qty":     trade["volume"],
				"avg_price":    trade["price"],
				"last_price":   trade["price"],
				"cost":         trade["cost"],
				"fee":          trade["fee"],
				"fee_ccy":      quoteFromPair(pair),
				"timestamp":    timestamp,
			}},
			"timestamp": timestamp,
		})
	}

	return out
}

func (account *PaperAccount) emit(frame map[string]any) {
	if frame == nil {
		return
	}

	for _, observer := range account.observers {
		select {
		case <-account.ctx.Done():
			return
		case observer <- frame:
		}
	}
}

func (account *PaperAccount) frameType() string {
	if account.synced {
		return "update"
	}

	return "snapshot"
}

type paperBalanceResponse struct {
	Balances map[string]map[string]any `json:"balances"`
	Mode     string                    `json:"mode"`
}

type paperOrdersResponse struct {
	Count      int              `json:"count"`
	Mode       string           `json:"mode"`
	OpenOrders []map[string]any `json:"open_orders"`
}

type paperHistoryResponse struct {
	Mode   string           `json:"mode"`
	Trades []map[string]any `json:"trades"`
}

func normalizePair(pair string) string {
	pair = strings.TrimSpace(pair)
	if pair == "" || strings.Contains(pair, "/") {
		return pair
	}

	quote := paperQuote()
	upper := strings.ToUpper(pair)
	if strings.HasSuffix(upper, quote) && len(pair) > len(quote) {
		return pair[:len(pair)-len(quote)] + "/" + quote
	}

	return pair
}

func quoteFromPair(pair string) string {
	normalized := normalizePair(pair)
	if _, quote, ok := strings.Cut(normalized, "/"); ok {
		return strings.ToUpper(strings.TrimSpace(quote))
	}

	return paperQuote()
}

func paperQuote() string {
	quote := strings.ToUpper(strings.TrimSpace(viper.GetString("market.quote_currency")))
	if quote == "" {
		return "USD"
	}

	return quote
}

func paperPair(symbol string) string {
	return strings.ReplaceAll(strings.TrimSpace(symbol), "/", "")
}

func stringField(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}
