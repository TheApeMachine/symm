package broker

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

/*
Desk is the link between the trader and the Kraken exchange. It opens and closes
positions on the trader's command and protects them with trailing stops. It makes
no entry decisions of its own; the only call it makes alone is bailing out of a
position whose stop has been breached. Durable stop snapshots live in the shared
tree; live protection state is bounded to pending entries, open stops, and the
latest quote per observed symbol.
*/
type Desk struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q[any]
	tree        *dmt.Tree
	privateBus  *qpool.BroadcastGroup
	subscribers []*qpool.BroadcastConsumer
	quote       string
	state       atomic.Pointer[deskState]
}

type deskState struct {
	quotes     map[string]tickerRow
	pending    map[string]pendingStop
	stoplosses map[string]*Stoploss
}

type tickerFrame struct {
	Channel string      `json:"channel"`
	Type    string      `json:"type"`
	Data    []tickerRow `json:"data"`
}

type tickerRow struct {
	Symbol    string  `json:"symbol"`
	Last      float64 `json:"last"`
	Bid       float64 `json:"bid"`
	Ask       float64 `json:"ask"`
	UpdatedAt int64   `json:"-"`
}

type pendingStop struct {
	Symbol  string
	Qty     float64
	Filled  float64
	Offset  float64
	ClOrdID string
}

type executionFrame struct {
	Data       []executionRow          `json:"data"`
	Executions map[string]executionRow `json:"executions"`
}

type executionRow struct {
	OrderID     string  `json:"order_id"`
	ClOrdID     string  `json:"cl_ord_id"`
	Symbol      string  `json:"symbol"`
	Side        string  `json:"side"`
	OrderType   string  `json:"order_type"`
	OrderStatus string  `json:"order_status"`
	ExecType    string  `json:"exec_type"`
	OrderQty    float64 `json:"order_qty"`
	LastQty     float64 `json:"last_qty"`
	LastPrice   float64 `json:"last_price"`
	AvgPrice    float64 `json:"avg_price"`
	CumQty      float64 `json:"cum_qty"`
}

func NewDesk(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) *Desk {
	ctx, cancel := context.WithCancel(ctx)

	desk := &Desk{
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		tree:       tree,
		privateBus: pool.CreateBroadcastGroup("kraken:private"),
		quote:      strings.ToUpper(viper.GetString("market.quote_currency")),
	}

	desk.state.Store(newDeskState())

	for _, channel := range []string{"ticker", "executions"} {
		desk.subscribers = append(desk.subscribers, pool.Subscribe(channel, desk.onMessage))
	}

	return desk
}

func newDeskState() *deskState {
	return &deskState{
		quotes:     make(map[string]tickerRow),
		pending:    make(map[string]pendingStop),
		stoplosses: make(map[string]*Stoploss),
	}
}

func (state *deskState) clone() *deskState {
	next := &deskState{
		quotes:     make(map[string]tickerRow, len(state.quotes)),
		pending:    make(map[string]pendingStop, len(state.pending)),
		stoplosses: make(map[string]*Stoploss, len(state.stoplosses)),
	}

	for key, value := range state.quotes {
		next.quotes[key] = value
	}
	for key, value := range state.pending {
		next.pending[key] = value
	}
	for key, value := range state.stoplosses {
		next.stoplosses[key] = value
	}

	return next
}

func (desk *Desk) snapshot() *deskState {
	state := desk.state.Load()
	if state == nil {
		panic(errnie.Err(errnie.Validation, "broker/Desk: uninitialized state", nil))
	}
	return state
}

func (desk *Desk) updateState(mut func(*deskState) error) error {
	for {
		current := desk.snapshot()
		next := current.clone()
		if err := mut(next); err != nil {
			return err
		}
		if desk.state.CompareAndSwap(current, next) {
			return nil
		}
	}
}

/*
onMessage will be called by the qpool.BroadcastGroup for every consumer
that has subscribed with a callback function.
*/
func (desk *Desk) onMessage(artifact *datura.Artifact) error {
	if artifact == nil {
		panic(errnie.Err(errnie.Validation, "broker/Desk: nil message artifact", nil))
	}

	destination, destinationErr := artifact.Destination()
	if destinationErr != nil {
		panic(errnie.Err(errnie.Validation, "broker/Desk: failed to get destination", destinationErr))
	}

	role, roleErr := artifact.Role()
	if roleErr != nil {
		panic(errnie.Err(errnie.Validation, "broker/Desk: failed to get role", roleErr))
	}

	switch role {
	case "ticker":
		if destination != "desk" {
			panic(errnie.Err(
				errnie.Validation,
				"broker/Desk: ticker destination must be desk",
				errors.New(destination),
			))
		}
		if err := desk.observeTicker(artifact); err != nil {
			panic(err)
		}
	case "executions":
		if err := desk.observeExecutions(artifact); err != nil {
			panic(err)
		}
	default:
		panic(errnie.Err(
			errnie.Validation,
			"broker/Desk: ignored role",
			errors.New(role),
		))
	}

	return nil
}

func (desk *Desk) observeTicker(artifact *datura.Artifact) error {
	frame, frameErr := tickerFromArtifact(artifact)
	if frameErr != nil {
		return frameErr
	}
	updatedAt := artifact.Timestamp()
	if updatedAt <= 0 {
		return errnie.Err(errnie.Validation, "broker/Desk: ticker artifact missing timestamp", nil)
	}

	for index := range frame.Data {
		frame.Data[index].UpdatedAt = updatedAt
	}

	if err := desk.updateState(func(next *deskState) error {
		for _, row := range frame.Data {
			next.quotes[row.Symbol] = row
		}
		return nil
	}); err != nil {
		return err
	}

	exits := make([]StoplossExit, 0)

	for _, row := range frame.Data {
		stoploss := desk.snapshot().stoplosses[row.Symbol]
		if stoploss == nil {
			continue
		}

		mark, markErr := row.exitMark()
		if markErr != nil {
			return markErr
		}

		exit, breached, ratchetErr := stoploss.Ratchet(mark)
		if ratchetErr != nil {
			return ratchetErr
		}

		desk.storeStop(stoploss)
		if breached {
			desk.deleteStop(row.Symbol, stoploss)
			exits = append(exits, exit)
		}
	}

	for _, exit := range exits {
		if err := desk.send(exit.Symbol, exit.Side, "market", exit.Qty, ""); err != nil {
			return err
		}
	}

	return nil
}

func tickerFromArtifact(artifact *datura.Artifact) (tickerFrame, error) {
	if artifact == nil {
		return tickerFrame{}, errnie.Err(errnie.Validation, "broker/Desk: nil ticker artifact", nil)
	}

	var frame tickerFrame
	if err := sonic.Unmarshal(artifact.DecryptPayload(), &frame); err != nil {
		return tickerFrame{}, errnie.Err(errnie.Validation, "broker/Desk: decode ticker", err)
	}

	if frame.Channel != "" && frame.Channel != "ticker" {
		return tickerFrame{}, errnie.Err(
			errnie.Validation,
			"broker/Desk: expected ticker channel, got "+frame.Channel,
			nil,
		)
	}
	if len(frame.Data) == 0 {
		return tickerFrame{}, errnie.Err(errnie.Validation, "broker/Desk: ticker frame has no rows", nil)
	}

	for _, row := range frame.Data {
		if row.Symbol == "" {
			return tickerFrame{}, errnie.Err(errnie.Validation, "broker/Desk: ticker row missing symbol", nil)
		}
	}

	return frame, nil
}

func (row tickerRow) entryMark() (float64, error) {
	if row.Ask <= 0 {
		return 0, errnie.Err(
			errnie.Validation,
			"broker/Desk: ticker row has no positive ask for "+row.Symbol,
			nil,
		)
	}
	if row.Bid > 0 && row.Ask < row.Bid {
		return 0, errnie.Err(
			errnie.Validation,
			"broker/Desk: crossed ticker quote for "+row.Symbol,
			nil,
		)
	}

	return row.Ask, nil
}

func (row tickerRow) exitMark() (float64, error) {
	if row.Bid <= 0 {
		return 0, errnie.Err(
			errnie.Validation,
			"broker/Desk: ticker row has no positive bid for "+row.Symbol,
			nil,
		)
	}
	if row.Ask > 0 && row.Ask < row.Bid {
		return 0, errnie.Err(
			errnie.Validation,
			"broker/Desk: crossed ticker quote for "+row.Symbol,
			nil,
		)
	}

	return row.Bid, nil
}

func (row tickerRow) freshEnough(now time.Time) error {
	if row.UpdatedAt <= 0 {
		return errnie.Err(
			errnie.Validation,
			"broker/Desk: ticker row missing update timestamp for "+row.Symbol,
			nil,
		)
	}

	maxAge := viper.GetDuration("trading.max_quote_age")
	if maxAge <= 0 {
		return nil
	}

	age := now.Sub(time.Unix(0, row.UpdatedAt).UTC())
	if age > maxAge {
		return errnie.Err(
			errnie.Validation,
			"broker/Desk: stale live quote for "+row.Symbol,
			nil,
		).With("age", age.String(), "max_age", maxAge.String())
	}

	return nil
}

func (desk *Desk) observeExecutions(artifact *datura.Artifact) error {
	rows, rowsErr := executionRows(artifact)
	if rowsErr != nil {
		return rowsErr
	}

	for _, row := range rows {
		if err := desk.observeExecution(row); err != nil {
			return err
		}
	}

	return nil
}

func (desk *Desk) observeExecution(row executionRow) error {
	clOrdID := strings.TrimSpace(row.ClOrdID)
	if clOrdID == "" {
		return errnie.Err(errnie.Validation, "broker/Desk: execution missing cl_ord_id", nil)
	}

	for {
		current := desk.snapshot()
		pending, ok := current.pending[clOrdID]
		if !ok {
			return nil
		}
		if strings.ToLower(row.Side) != "buy" {
			return errnie.Err(errnie.Validation, "broker/Desk: pending entry filled with non-buy side", nil)
		}
		if row.Symbol != pending.Symbol {
			return errnie.Err(errnie.Validation, "broker/Desk: fill symbol does not match pending entry", nil)
		}
		if !row.fillConfirmed() {
			return errnie.Err(
				errnie.Validation,
				"broker/Desk: pending entry execution is not a fill confirmation",
				nil,
			)
		}

		qty := row.fillQty()
		if row.CumQty <= 0 && pending.Filled > 0 {
			qty += pending.Filled
		}
		if qty <= 0 {
			return errnie.Err(errnie.Validation, "broker/Desk: fill has non-positive quantity", nil)
		}
		if pending.Filled > 0 && qty < pending.Filled {
			return errnie.Err(errnie.Validation, "broker/Desk: fill quantity regressed", nil)
		}

		price := row.fillPrice()
		if price <= 0 {
			return errnie.Err(errnie.Validation, "broker/Desk: fill has non-positive price", nil)
		}

		stoploss := current.stoplosses[pending.Symbol]
		if stoploss == nil {
			stoploss = NewStoploss(pending.Symbol, qty, price, pending.Offset)
		} else if coverErr := stoploss.Cover(qty, price); coverErr != nil {
			return coverErr
		}

		next := current.clone()
		next.stoplosses[pending.Symbol] = stoploss
		pending.Filled = qty
		if row.fillComplete(pending.Qty, qty) {
			delete(next.pending, clOrdID)
		} else {
			next.pending[clOrdID] = pending
		}

		if desk.state.CompareAndSwap(current, next) {
			desk.storeStop(stoploss)
			return nil
		}
	}
}

func executionRows(artifact *datura.Artifact) ([]executionRow, error) {
	if artifact == nil {
		return nil, errnie.Err(errnie.Validation, "broker/Desk: nil executions artifact", nil)
	}

	var frame executionFrame
	if err := sonic.Unmarshal(artifact.DecryptPayload(), &frame); err != nil {
		return nil, errnie.Err(errnie.Validation, "broker/Desk: decode executions", err)
	}

	rows := make([]executionRow, 0, len(frame.Data)+len(frame.Executions))
	rows = append(rows, frame.Data...)
	for _, row := range frame.Executions {
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return nil, errnie.Err(errnie.Validation, "broker/Desk: executions frame has no rows", nil)
	}

	return rows, nil
}

func (row executionRow) fillQty() float64 {
	switch {
	case row.CumQty > 0:
		return row.CumQty
	case row.LastQty > 0:
		return row.LastQty
	default:
		return row.OrderQty
	}
}

func (row executionRow) fillPrice() float64 {
	if row.AvgPrice > 0 {
		return row.AvgPrice
	}
	return row.LastPrice
}

/*
fillConfirmed reports whether the execution row is an actual trade/fill update,
not merely an order lifecycle update. The desk arms protection only after this
confirmation exists.
*/
func (row executionRow) fillConfirmed() bool {
	status := strings.ToLower(strings.TrimSpace(row.OrderStatus))
	execType := strings.ToLower(strings.TrimSpace(row.ExecType))

	if execType != "" && execType != "trade" {
		return false
	}
	if status != "" && status != "filled" && status != "partially_filled" && status != "partial" {
		return false
	}

	return execType == "trade" || status != ""
}

func (row executionRow) fillComplete(expectedQty float64, filledQty float64) bool {
	if strings.ToLower(strings.TrimSpace(row.OrderStatus)) == "filled" {
		return true
	}
	return expectedQty > 0 && filledQty >= expectedQty
}

/*
Update dispatches the actions the trader chose this tick. Ticker messages drive
stop ratcheting through onMessage, so the desk never scans the tree for live
marks.
*/
func (desk *Desk) Update(actions []*datura.Artifact) error {
	for _, action := range actions {
		if err := desk.execute(action); err != nil {
			return err
		}
	}

	return nil
}

/*
execute turns one trader action into an exchange order. A buy creates a pending
entry that is protected only after the execution feed confirms the fill; a sell
closes the position and retires the stop.
*/
func (desk *Desk) execute(action *datura.Artifact) error {
	if action == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "desk: nil action", nil))
	}

	side := errnie.Does(func() (string, error) { return action.Role() }).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"failed to get role",
			err,
		))
	}).Value()

	symbol := errnie.Does(func() (string, error) { return action.Scope() }).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"failed to get scope",
			err,
		))
	}).Value()

	if symbol == "" || !slices.Contains([]string{"buy", "sell"}, side) {
		return errnie.Error(errnie.Err(
			errnie.Validation, "desk: action missing symbol or side: "+symbol+"/"+side, nil,
		))
	}

	orderType := datura.Peek[string](action, "type")
	qty := datura.Peek[float64](action, "quantity")
	clOrdID := datura.Peek[string](action, "cl_ord_id")
	offset := datura.Peek[float64](action, "offset")

	// The trader sizes entries by risk fraction; the desk turns that into a
	// quantity here, where the live mark and free quote balance are known. An
	// explicit quantity (e.g. on exits) is used as-is.
	if side == "buy" {
		if fraction := datura.Peek[float64](action, "fraction"); fraction > 0 {
			sizedQty, sizeErr := desk.sizeBuy(symbol, fraction)
			if sizeErr != nil {
				return sizeErr
			}
			qty = sizedQty
		}
	} else if qty > 0 {
		// Exits carry an explicit quantity; round it to the exchange increment
		// so the sell is not rejected for sub-increment precision. No minimum
		// guard — an exit must always be able to flatten the position.
		roundedQty, roundErr := desk.roundQuantity(symbol, qty)
		if roundErr != nil {
			return roundErr
		}
		qty = roundedQty
	}

	if qty <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation, "desk: non-positive quantity for "+symbol, nil,
		))
	}

	if side == "buy" {
		if offset <= 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation, "desk: entry for "+symbol+" has no stop offset", nil,
			))
		}

		var idErr error
		clOrdID, idErr = orderID(action, clOrdID)
		if idErr != nil {
			return idErr
		}

		if err := desk.addPending(clOrdID, pendingStop{
			Symbol:  symbol,
			Qty:     qty,
			Offset:  offset,
			ClOrdID: clOrdID,
		}); err != nil {
			return err
		}
	}

	if err := desk.send(symbol, side, orderType, qty, clOrdID); err != nil {
		if side == "buy" {
			desk.deletePending(clOrdID)
		}
		return err
	}

	if side == "sell" {
		desk.retireStop(symbol)
		return nil
	}

	return nil
}

func (desk *Desk) addPending(clOrdID string, pending pendingStop) error {
	return desk.updateState(func(next *deskState) error {
		next.pending[clOrdID] = pending
		return nil
	})
}

func (desk *Desk) deletePending(clOrdID string) {
	_ = desk.updateState(func(next *deskState) error {
		delete(next.pending, clOrdID)
		return nil
	})
}

func orderID(_ *datura.Artifact, current string) (string, error) {
	if strings.TrimSpace(current) != "" {
		return current, nil
	}

	return "", errnie.Err(errnie.Validation, "desk: buy action missing cl_ord_id", nil)
}

/*
send emits an add_order to the private bus, retrying a few times before giving up
and logging the failure.
*/
func (desk *Desk) send(
	symbol, side, orderType string, qty float64, clOrdID string,
) error {
	order := datura.Acquire(
		"broker", datura.APPJSON,
	).WithDestination(
		"kraken:private",
	).WithRole(
		"orders",
	).WithPayload(datura.Map[any]{
		"method": "add_order",
		"params": datura.Map[any]{
			"symbol":     symbol,
			"side":       side,
			"order_type": orderType,
			"order_qty":  qty,
			"cl_ord_id":  clOrdID,
		},
	}.Marshal())

	var sendErr error

	for range 3 {
		if err := desk.privateBus.Send(order); err == nil {
			return nil
		} else {
			sendErr = err
		}
	}

	return errnie.Error(errnie.Err(
		errnie.Validation, "desk: failed to send order for "+symbol, sendErr,
	))
}

func (desk *Desk) markFor(symbol string) (float64, error) {
	row, ok := desk.snapshot().quotes[symbol]
	if !ok {
		return 0, errnie.Err(errnie.Validation, "desk: no live quote for "+symbol, nil)
	}

	if err := row.freshEnough(time.Now().UTC()); err != nil {
		return 0, err
	}

	return row.entryMark()
}

func (desk *Desk) putStop(stoploss *Stoploss) error {
	if stoploss == nil {
		return errnie.Err(errnie.Validation, "desk: nil stoploss", nil)
	}

	if err := desk.updateState(func(next *deskState) error {
		next.stoplosses[stoploss.Symbol] = stoploss
		return nil
	}); err != nil {
		return err
	}

	desk.storeStop(stoploss)
	return nil
}

func (desk *Desk) deleteStop(symbol string, stoploss *Stoploss) {
	_ = desk.updateState(func(next *deskState) error {
		if current := next.stoplosses[symbol]; current == stoploss {
			delete(next.stoplosses, symbol)
		}
		return nil
	})
}

func (desk *Desk) retireStop(symbol string) {
	stoploss := desk.snapshot().stoplosses[symbol]
	if stoploss == nil {
		return
	}

	if err := stoploss.Close(); err != nil {
		panic(errnie.Err(errnie.Validation, "desk: close stoploss", err))
	}
	desk.storeStop(stoploss)
	desk.deleteStop(symbol, stoploss)
}

func (desk *Desk) storeStop(stoploss *Stoploss) {
	if stoploss == nil {
		panic(errnie.Err(errnie.Validation, "desk: nil stoploss", nil))
	}

	desk.tree.InsertArtifact(stoplossKey(stoploss.Symbol), stoploss.Artifact())
}

func stoplossKey(symbol string) []byte {
	return []byte("stoploss/" + symbol)
}

func (desk *Desk) Close() error {
	desk.cancel()
	return nil
}
