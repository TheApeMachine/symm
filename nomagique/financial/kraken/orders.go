package kraken

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
)

/* OrdersServer owns authenticated order transport, never a second position ledger. */
type OrdersServer struct {
	api   *spot.REST
	terms Terms
}

/* NewOrders creates an idle transport with no submitted or polled orders. */
func NewOrders() *OrdersServer { return &OrdersServer{api: spot.NewREST()} }

/* Write receives the existing graph's credential and normalization capabilities. */
func (server *OrdersServer) Write(ctx context.Context, call Orders_write) error {
	public, err := call.Args().PublicKey()

	if err != nil {
		return errnie.Error(err)
	}
	private, err := call.Args().PrivateKey()

	if err != nil {
		return errnie.Error(err)
	}

	if len(public) == 0 || len(private) == 0 || !call.Args().Terms().IsValid() || !call.Args().TermsReady() {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken orders: credentials and initialized venue terms are required", nil))
	}

	if server.terms.IsValid() {
		server.terms.Release()
	}
	server.api.PublicKey, server.api.PrivateKey = string(public), string(private)
	server.terms = call.Args().Terms().AddRef()
	return nil
}

/* Done reports transport configuration without contacting the exchange. */
func (server *OrdersServer) Done(ctx context.Context, call Orders_done) error {
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	result.SetReady(server.terms.IsValid() && server.api.PublicKey != "" && server.api.PrivateKey != "")
	return nil
}

/* Shutdown releases the capability retained at configuration. */
func (server *OrdersServer) Shutdown() {
	if server.terms.IsValid() {
		server.terms.Release()
	}
}

/* Submit sends one normalized market order, with caller-owned stable identity. */
func (server *OrdersServer) Submit(ctx context.Context, call Orders_submit) error {
	request, err := call.Args().Request()

	if err != nil {
		return errnie.Error(err)
	}
	symbol, err := request.Symbol()

	if err != nil {
		return errnie.Error(err)
	}
	quantity, err := request.Quantity()

	if err != nil {
		return errnie.Error(err)
	}
	side, err := request.Side()

	if err != nil {
		return errnie.Error(err)
	}
	identity, err := request.ClientId()

	if err != nil {
		return errnie.Error(err)
	}

	if identity == "" || (side != "buy" && side != "sell") || !server.terms.IsValid() {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken orders: stable client identity, buy/sell side and venue terms are required", nil))
	}
	future, release := server.terms.Normalize(ctx, func(params Terms_normalize_Params) error {
		if err := params.SetSymbol(symbol); err != nil {
			return err
		}
		return params.SetQuantity(quantity)
	})
	defer release()
	normalized, err := future.Struct()

	if err != nil {
		return errnie.Error(err)
	}
	symbol, err = normalized.Symbol()

	if err != nil {
		return errnie.Error(err)
	}
	quantity, err = normalized.Quantity()

	if err != nil {
		return errnie.Error(err)
	}
	if err := server.prepare(ctx); err != nil {
		return err
	}
	response, err := server.api.AddOrder(&spot.AddOrderRequest{ClOrdId: identity, Pair: symbol, Type: side, OrderType: "market", Volume: quantity})

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "kraken orders: submit requires reconciliation before retry", err))
	}

	if len(response.Result.ID) != 1 || response.Result.ID[0] == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken orders: venue omitted unique order identity", nil))
	}
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	return errnie.Error(result.SetId(response.Result.ID[0]))
}

/* Inspect returns authoritative cumulative fills for one acknowledged order. */
func (server *OrdersServer) Inspect(ctx context.Context, call Orders_inspect) error {
	identity, err := call.Args().Id()

	if err != nil {
		return errnie.Error(err)
	}

	if identity == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken orders: query requires order identity", nil))
	}
	if err := server.prepare(ctx); err != nil {
		return err
	}
	response, err := server.api.QueryOrders(&spot.QueryOrdersRequest{TxID: identity})

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "kraken orders: query cumulative execution", err))
	}
	order, present := response.Result[identity]

	if !present {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken orders: incomplete cumulative execution response", nil))
	}
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	execution, err := result.NewOrder()

	if err != nil {
		return errnie.Error(err)
	}
	return server.emit(execution, identity, order.Order)
}

/* Cancel requests cancellation; only a subsequent Inspect confirms its fills and status. */
func (server *OrdersServer) Cancel(ctx context.Context, call Orders_cancel) error {
	identity, err := call.Args().Id()

	if err != nil {
		return errnie.Error(err)
	}

	if identity == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken orders: cancellation requires order identity", nil))
	}
	if err := server.prepare(ctx); err != nil {
		return err
	}
	response, err := server.api.CancelOrder(&spot.CancelOrderRequest{TxID: identity})

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "kraken orders: cancellation", err))
	}
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	result.SetCount(uint32(response.Result.Count))
	return nil
}

/*
	Find resolves an uncertain submission by its stable client identity. Absence

is observable and never authorizes automatic resubmission.
*/
func (server *OrdersServer) Find(ctx context.Context, call Orders_find) error {
	client, err := call.Args().ClientId()

	if err != nil {
		return errnie.Error(err)
	}

	if client == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken orders: client identity is required", nil))
	}
	if err := server.prepare(ctx); err != nil {
		return err
	}
	opened, err := server.api.OpenOrders(&spot.OpenOrdersRequest{ClOrdID: client})

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "kraken orders: reconcile open client identity", err))
	}
	if err := server.prepare(ctx); err != nil {
		return err
	}
	closed, err := server.api.ClosedOrders(&spot.ClosedOrdersRequest{ClOrdID: client})

	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "kraken orders: reconcile closed client identity", err))
	}
	retained := make(map[string]*spot.Order)
	for identity, order := range opened.Result.Open {
		if order.ClOrdID == client {
			retained[identity] = &order
		}
	}
	for identity, order := range closed.Result.Closed {
		if order.Order != nil && order.ClOrdID == client {
			retained[identity] = order.Order
		}
	}

	if len(retained) > 1 {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken orders: client identity resolved to multiple orders", nil))
	}
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	result.SetFound(len(retained) == 1)
	for identity, order := range retained {
		execution, err := result.NewOrder()
		if err != nil {
			return errnie.Error(err)
		}
		return server.emit(execution, identity, order)
	}
	return nil
}

/* emit copies authoritative cumulative execution without repricing it. */
func (server *OrdersServer) emit(execution Execution, identity string, order *spot.Order) error {
	if order == nil || order.Description == nil || order.VolumeExecuted == nil || order.Cost == nil || order.Fee == nil || order.Price == nil || order.Status == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken orders: incomplete cumulative execution response", nil))
	}
	for _, err := range []error{execution.SetId(identity), execution.SetClientId(order.ClOrdID), execution.SetSymbol(order.Description.Pair), execution.SetSide(order.Description.Type), execution.SetStatus(order.Status), execution.SetQuantity(order.VolumeExecuted.String()), execution.SetCost(order.Cost.String()), execution.SetFee(order.Fee.String()), execution.SetAveragePrice(order.Price.String())} {
		if err != nil {
			return errnie.Error(err)
		}
	}
	return nil
}

/* prepare obtains a nonce from the same Terms owner used for account fees. */
func (server *OrdersServer) prepare(ctx context.Context) error {
	if !server.terms.IsValid() {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken orders: configured venue terms are required", nil))
	}
	future, release := server.terms.Nonce(ctx, nil)
	defer release()
	result, err := future.Struct()
	if err != nil {
		return errnie.Error(err)
	}
	nonce, err := result.Value()
	if err != nil {
		return errnie.Error(err)
	}
	if nonce == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "kraken orders: venue signer omitted nonce", nil))
	}
	server.api.Nonce = func() string { return nonce }
	return nil
}
