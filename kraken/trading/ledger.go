package trading

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
)

const ledgerSubscriberID = "kraken/trading:ledger"

/*
OrderResult is the resolved outcome for one client order id.
*/
type OrderResult struct {
	ClOrdID string
	Success bool
	Error   string
	OrderID string
}

type pendingOrder struct {
	result chan OrderResult
	timer  *time.Timer
}

/*
Ledger multiplexes private order acknowledgements onto per-order futures.
*/
type Ledger struct {
	ctx        context.Context
	cancel     context.CancelFunc
	raw        *qpool.BroadcastGroup
	orders     *qpool.BroadcastGroup
	mu         sync.Mutex
	pending    map[string]*pendingOrder
	tripped    atomic.Bool
	subscriber *qpool.Subscriber
	resultPool sync.Pool
}

func NewLedger(
	ctx context.Context,
	pool *qpool.Q,
	orders *qpool.BroadcastGroup,
) (*Ledger, error) {
	ctx, cancel := context.WithCancel(ctx)

	ledger := &Ledger{
		ctx:     ctx,
		cancel:  cancel,
		raw:     pool.CreateBroadcastGroup("raw", 10*time.Millisecond),
		orders:  orders,
		pending: make(map[string]*pendingOrder),
	}

	ledger.resultPool.New = func() any {
		return make(chan OrderResult, 1)
	}

	ledger.subscriber = ledger.raw.Subscribe(ledgerSubscriberID, 256)

	return ledger, errnie.Error(errnie.Require(map[string]any{
		"ctx":        ledger.ctx,
		"cancel":     ledger.cancel,
		"raw":        ledger.raw,
		"orders":     ledger.orders,
		"subscriber": ledger.subscriber,
	}))
}

func AckTimeout() time.Duration {
	timeout := viper.GetDuration("trading.order_ack_timeout")

	if timeout <= 0 {
		return 500 * time.Millisecond
	}

	return timeout
}

func (ledger *Ledger) Run() error {
	defer ledger.raw.Unsubscribe(ledger.subscriber.ID)

	for {
		select {
		case <-ledger.ctx.Done():
			return ledger.ctx.Err()
		case message, ok := <-ledger.subscriber.Incoming:
			if !ok {
				return ledger.ctx.Err()
			}

			if message == nil || message.Value == nil {
				continue
			}

			ledger.handle(message.Value)
		}
	}
}

func (ledger *Ledger) Halted() bool {
	return ledger.tripped.Load()
}

func (ledger *Ledger) RawGroup() *qpool.BroadcastGroup {
	return ledger.raw
}

func (ledger *Ledger) ReleaseResult(resultCh chan OrderResult) {
	select {
	case <-resultCh:
	default:
	}

	ledger.resultPool.Put(resultCh)
}

func (ledger *Ledger) Unregister(clOrdID string) {
	ledger.mu.Lock()
	entry, ok := ledger.pending[clOrdID]

	if !ok {
		ledger.mu.Unlock()

		return
	}

	delete(ledger.pending, clOrdID)
	ledger.mu.Unlock()

	if entry.timer != nil {
		entry.timer.Stop()
	}

	ledger.ReleaseResult(entry.result)
}

func (ledger *Ledger) Register(clOrdID string) chan OrderResult {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	resultCh := ledger.resultPool.Get().(chan OrderResult)

	select {
	case <-resultCh:
	default:
	}

	entry := &pendingOrder{
		result: resultCh,
	}

	entry.timer = time.AfterFunc(AckTimeout(), func() {
		ledger.resolve(clOrdID, OrderResult{
			ClOrdID: clOrdID,
			Success: false,
			Error:   "order ack timeout",
		})
		ledger.trip()
	})

	ledger.pending[clOrdID] = entry

	return resultCh
}

func (ledger *Ledger) trip() {
	if ledger.tripped.Swap(true) {
		return
	}

	ledger.orders.Send(&qpool.QValue[any]{
		Type: public.OrdersChannel,
		Value: map[string]any{
			"method": MethodCancelAll,
			"params": CancelAllParams{},
		},
	})
}

func (ledger *Ledger) handle(value any) {
	if envelope, ok := value.(public.SocketMessage); ok {
		if envelope.Channel == public.ExecutionsChannel {
			executions := errnie.Does(func() ([]user.Execution, error) {
				return user.DecodeExecutions(&envelope)
			}).Or(func(err error) {
				errnie.Error(err)
			}).Value()

			for _, execution := range executions {
				ledger.handleExecution(execution)
			}
		}

		return
	}

	frame, ok := value.(map[string]any)

	if !ok {
		return
	}

	method, _ := frame["method"].(string)

	if method != MethodAddOrder {
		return
	}

	success, _ := frame["success"].(bool)
	errorText, _ := frame["error"].(string)

	resultFrame, _ := frame["result"].(map[string]any)

	if resultFrame == nil {
		return
	}

	clOrdID, _ := resultFrame["cl_ord_id"].(string)
	orderID, _ := resultFrame["order_id"].(string)

	if clOrdID == "" {
		return
	}

	result := OrderResult{
		ClOrdID: clOrdID,
		Success: success,
		Error:   errorText,
		OrderID: orderID,
	}

	ledger.resolve(clOrdID, result)
}

func (ledger *Ledger) handleExecution(execution user.Execution) {
	if execution.ClOrdID == "" {
		return
	}

	if execution.ExecType == "rejected" || execution.OrderStatus == "rejected" {
		ledger.resolve(execution.ClOrdID, OrderResult{
			ClOrdID: execution.ClOrdID,
			Success: false,
			Error:   execution.OrderStatus,
			OrderID: execution.OrderID,
		})

		return
	}

	if execution.ExecType == "" && execution.OrderStatus == "" {
		return
	}

	ledger.resolve(execution.ClOrdID, OrderResult{
		ClOrdID: execution.ClOrdID,
		Success: true,
		OrderID: execution.OrderID,
	})
}

func (ledger *Ledger) resolve(clOrdID string, result OrderResult) {
	ledger.mu.Lock()
	entry, ok := ledger.pending[clOrdID]

	if !ok {
		ledger.mu.Unlock()

		return
	}

	delete(ledger.pending, clOrdID)
	ledger.mu.Unlock()

	if entry.timer != nil {
		entry.timer.Stop()
	}

	entry.result <- result
}

func (ledger *Ledger) Close() error {
	ledger.cancel()

	return nil
}
