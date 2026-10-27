package broker

import (
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Desk owns open positions: it sizes entries from the book (cash budget,
touch-bid carry divisor, and ask walk), works them in child market
orders, takes the learned exit, runs the capacity monitor, accumulates venue
fills beside their shadow fills, and reports each completed round trip.
*/
type Desk struct {
	*runtime.System
	mu        sync.Mutex
	transport Transport
	price     *Price
	balance   *Balance
	history   *History
	positions *Cache
	virtuals  *Cache
	// virtualArchive is every settled virtual round trip (S3 closed records
	// at boot and each Exit). Train Drop clears the live cache only; the
	// archive keeps closed lots visible to the dashboard.
	virtualArchive *sync.Map
	pendingTrain   *sync.Map
	helloMu        sync.Mutex
	lastHello      time.Time
	helloArmed     bool
}

const helloMinGap = 250 * time.Millisecond

func NewDesk(
	ctx context.Context,
	transport Transport,
	price *Price,
	balance *Balance,
) *Desk {
	desk := &Desk{
		transport:      transport,
		balance:        balance,
		history:        NewHistory(ctx),
		price:          price,
		positions:      NewCache(ctx),
		virtuals:       NewCache(ctx),
		virtualArchive: &sync.Map{},
		pendingTrain:   &sync.Map{},
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)

	for key, val := range map[string]*Cache{
		"position": desk.positions,
		"virtual":  desk.virtuals,
	} {
		for value := range desk.history.Restore(key) {
			fields := strings.Split(value.Prefix, "/")
			if len(fields) < 5 {
				continue
			}

			role := fields[1]

			position := &Position{}
			value.Lookup(position, key)

			if value.Err != nil {
				// A frame that does not decode is already recorded on the
				// value; report it and keep restoring the rest.
				desk.Error(value.Err)
				continue
			}

			if position.Symbol == "" {
				continue
			}

			// History JSON does not carry the live price book. Without it,
			// later marks never reprice and a lot in profit never exits.
			position.price = desk.price
			position.transport = desk.transport
			position.onSettled = desk.onPositionSettled

			switch role {
			case "open":
				if position.ClosedAt != nil && !position.ClosedAt.IsZero() {
					continue
				}

				val.Restore(position)

				if key == "virtual" {
					types.SetVirtualOpen(position.Symbol, true)
				}
			case "closed":
				if key == "virtual" && position.ID != "" {
					desk.archiveVirtual(position)
				}
			}
		}
	}

	desk.Transition(runtime.READY)
	return desk
}

/*
ArchiveVirtual keeps a settled virtual lot visible after the live cache drops it.
*/
func (desk *Desk) ArchiveVirtual(position *Position) {
	if desk.virtualArchive == nil {
		desk.virtualArchive = &sync.Map{}
	}

	desk.archiveVirtual(position)
}

func (desk *Desk) archiveVirtual(position *Position) {
	if desk == nil || position == nil || position.ID == "" {
		return
	}

	desk.virtualArchive.Store(position.ID, position)
}

/*
publishLots is the positions snapshot: open paper and virtual lots, any
virtual still on the closed side awaiting Train, and every archived
virtual close loaded from S3 or recorded at Exit.
*/
func (desk *Desk) publishLots() []*Position {
	seen := make(map[string]struct{})
	lots := make([]*Position, 0)

	add := func(position *Position) {
		if position == nil || position.ID == "" {
			return
		}

		if _, ok := seen[position.ID]; ok {
			return
		}

		seen[position.ID] = struct{}{}
		lots = append(lots, position)
	}

	for _, position := range desk.positions.All(true) {
		add(position)
	}

	for _, position := range desk.virtuals.All(true) {
		add(position)
	}

	for _, position := range desk.virtuals.Closed() {
		add(position)
	}

	desk.virtualArchive.Range(func(_, value any) bool {
		add(value.(*Position))
		return true
	})

	slices.SortFunc(lots, func(a, b *Position) int {
		return strings.Compare(a.Symbol, b.Symbol)
	})

	return lots
}

/*
Lots is the desk's round-trip set for the epoch tape: everything Hello
publishes, plus paper closes still on the desk. Paper closes leave the
open cache before the venue fills them, and Hello does not publish that
side.
*/
func (desk *Desk) Lots() []*Position {
	if desk == nil {
		return nil
	}

	lots := desk.publishLots()
	seen := make(map[string]struct{}, len(lots))

	for _, position := range lots {
		if position != nil && position.ID != "" {
			seen[position.ID] = struct{}{}
		}
	}

	for _, position := range desk.positions.Closed() {
		if position == nil || position.ID == "" {
			continue
		}

		if _, ok := seen[position.ID]; ok {
			continue
		}

		lots = append(lots, position)
	}

	return lots
}

/*
Hello is a callback for the ui.Hub that fires when a new WebSocket
connection is established, and we want to update the client with
some initial data.
*/
func (desk *Desk) Hello() {
	if desk == nil {
		return
	}

	desk.helloMu.Lock()
	wait := time.Since(desk.lastHello)

	if wait < helloMinGap {
		if !desk.helloArmed {
			desk.helloArmed = true
			time.AfterFunc(helloMinGap-wait, desk.flushHello)
		}

		desk.helloMu.Unlock()
		return
	}

	desk.lastHello = time.Now()
	desk.helloMu.Unlock()
	desk.writeHello()
}

func (desk *Desk) flushHello() {
	if desk == nil {
		return
	}

	desk.helloMu.Lock()
	desk.helloArmed = false
	desk.lastHello = time.Now()
	desk.helloMu.Unlock()
	desk.writeHello()
}

func (desk *Desk) writeHello() {
	io.Copy(ui.UI, data.NewValue("positions", "update", "all").Append(
		desk.publishLots(), "positions",
	))

	if desk.balance.value != nil {
		io.Copy(ui.UI, desk.balance.value)
	}
}

func (desk *Desk) cacheFor(virtual bool) (*Cache, string) {
	if virtual {
		return desk.virtuals, "virtual"
	}

	return desk.positions, "position"
}

/*
OpenPositions returns the open positions in the desk.
*/
func (desk *Desk) OpenPositions(virtual bool) []*Position {
	cache, _ := desk.cacheFor(virtual)
	return cache.All(true)
}

func (desk *Desk) Position(symbol string, virtual bool) *Position {
	cache, _ := desk.cacheFor(virtual)
	return cache.Get(symbol, true)
}

/*
ClosedVirtuals returns virtual lots settled this tick and still on the closed
side of the live cache, awaiting promotion scoring.
*/
func (desk *Desk) ClosedVirtuals() []*Position {
	if desk == nil {
		return nil
	}

	return desk.virtuals.Closed()
}

/*
TakePendingTrain returns a virtual lot closed on a mark exit since the last
take. Evaluate consumes it for path threshold updates.
*/
func (desk *Desk) TakePendingTrain(symbol string) (*Position, bool) {
	if desk == nil || symbol == "" {
		return nil, false
	}

	val, ok := desk.pendingTrain.LoadAndDelete(symbol)
	if !ok {
		return nil, false
	}

	position, ok := val.(*Position)
	return position, ok && position != nil
}

/*
PendingTrainSymbols lists pairs waiting for Evaluate to score a mark exit.
*/
func (desk *Desk) PendingTrainSymbols() []string {
	if desk == nil {
		return nil
	}

	out := make([]string, 0)

	desk.pendingTrain.Range(func(key, _ any) bool {
		if symbol, ok := key.(string); ok && symbol != "" {
			out = append(out, symbol)
		}

		return true
	})

	slices.Sort(out)
	return out
}

func (desk *Desk) onPositionSettled(position *Position) {
	if desk == nil || position == nil {
		return
	}

	desk.mu.Lock()
	defer desk.mu.Unlock()

	if err := desk.releaseLocked(position); err != nil {
		errnie.Error(err)
	}
}

/*
Update marks open positions against the trade price. Called from the workspace
tick handler — the desk sits off the trade ingress path, so this is where
marks become live.

The trade batch is the changed set. Those lots are the ones published. Every
other open lot is repriced from the book so an exit still sees a live mark,
and that reprice stays on the desk.
*/
func (desk *Desk) Update(trades []kraken.TradeData) {
	if len(trades) == 0 {
		return
	}

	latest := make(map[string]*decimal.Decimal, len(trades))
	for i := range trades {
		latest[trades[i].Symbol] = &trades[i].Price
	}

	var changed []*Position
	seen := make(map[string]struct{})

	note := func(position *Position) {
		if position == nil || position.ID == "" {
			return
		}

		if _, ok := seen[position.ID]; ok {
			return
		}

		seen[position.ID] = struct{}{}
		changed = append(changed, position)
	}

	for _, cache := range []*Cache{desk.positions, desk.virtuals} {
		for _, position := range cache.All(true) {
			if position == nil || position.Symbol == "" {
				continue
			}

			if mark, ok := latest[position.Symbol]; ok {
				position.Update(mark)
				note(position)
			} else if desk.repriceOpenFromBook(position) {
				note(position)
			}
		}
	}

	desk.publishChanged(changed)
}

/*
RepriceFromBook refreshes one open lot from the live book. Used when level3
moves the mark before the next trade print on that pair.
*/
func (desk *Desk) RepriceFromBook(symbol string) bool {
	if desk == nil || symbol == "" {
		return false
	}

	changed := false

	for _, val := range []*Cache{
		desk.positions, desk.virtuals,
	} {
		position := val.Get(symbol, true)

		if position == nil {
			continue
		}

		if desk.repriceOpenFromBook(position) {
			changed = true
		}
	}

	return changed
}

func (desk *Desk) repriceOpenFromBook(position *Position) bool {
	if desk == nil || position == nil || position.Symbol == "" {
		return false
	}

	mark := desk.price.CurrentMark(position.Symbol)
	if mark == nil {
		return false
	}

	position.Update(mark)
	return true
}

/*
publishChanged writes one frame per lot. The page keeps every other lot it
already has, keyed by id, so a quiet position stays on screen.
*/
func (desk *Desk) publishChanged(positions []*Position) {
	if desk == nil || len(positions) == 0 || ui.UI == nil || !ui.UI.Active() {
		return
	}

	for _, position := range positions {
		if position == nil || position.ID == "" {
			continue
		}

		io.Copy(ui.UI, data.NewValue("positions", "update", position.ID).Append(
			position, "position",
		))
	}
}

/*
Has reports whether the desk holds an open position (paper or virtual)
for the symbol.
*/
func (desk *Desk) Has(symbol string, virtual bool) bool {
	cache, _ := desk.cacheFor(virtual)
	return cache.Has(symbol)
}

/*
Apply an execution to the desk.
*/
func (desk *Desk) Apply(executions *kraken.Execution) {
	for _, execution := range executions.Data {
		position := desk.positions.Get(execution.Symbol, true)

		if position == nil {
			position = desk.positions.Get(execution.Symbol, false)
		}

		if position == nil {
			position = desk.virtuals.Get(execution.Symbol, true)
		}

		if position == nil {
			continue
		}

		position.Apply(execution)

		// A paper lot whose sell filled is fully settled: retire it from
		// the desk so it cannot shadow the next position on the symbol.
		if !position.Virtual && position.ClosedAt != nil {
			desk.positions.Remove(position.Symbol)
		}
	}
}

/*
Enter opens a lot on one book. The venue book and the virtual book are
independent: a learning sample does not occupy the venue slot, and a
venue lot does not occupy the learning slot. A second lot on the same
book is rejected.
*/
func (desk *Desk) Enter(
	symbol string, path string, virtual bool, pathFound bool,
) error {
	cache, key := desk.cacheFor(virtual)

	if cache.Has(symbol) {
		return errnie.Err(
			errnie.TooManyRequests,
			"[desk.Enter] "+key+" already exists",
			nil,
		)
	}

	position := NewPosition(
		symbol, path, virtual, desk.price, desk.transport,
		desk.onPositionSettled,
	)

	position.PathFound = pathFound

	if desk.price.Status() != runtime.READY {
		return errnie.Err(
			errnie.NotFound,
			"[desk.Enter] price is not ready for "+symbol,
			nil,
		)
	}

	err := position.Enter(path)

	if err != nil {
		return errnie.Error(err)
	}

	// The desk owns the position from the moment it enters: register it in
	// the right cache so ticks mark it and Hello publishes it immediately.
	// Without this only a restart (history restore) would ever surface a
	// live entry.
	cache.Add(position.Symbol, position)

	if virtual {
		types.SetVirtualOpen(symbol, true)
	}

	if err := desk.history.Store(data.NewValue(
		key, "open", symbol,
	).Append(
		position, "position",
	)); err != nil {
		cache.Drop(position.Symbol)
		if virtual {
			types.SetVirtualOpen(symbol, false)
		}
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Enter] failed to store position",
			err,
		))
	}

	// The entry is live. Publish this lot; the rest of the book is already
	// on the page from the last snapshot or its own updates.
	desk.publishChanged([]*Position{position})

	return nil
}

/*
Exit a position and report what settled.
*/
func (desk *Desk) Exit(
	symbol string, path string, virtual bool,
) error {
	cache, _ := desk.cacheFor(virtual)
	position := cache.Get(symbol, true)

	if position == nil {
		return errnie.Err(
			errnie.NotFound,
			"[desk.Exit] position not found",
			nil,
		)
	}

	if err := position.Exit(path); err != nil {
		return err
	}

	desk.mu.Lock()
	defer desk.mu.Unlock()

	return desk.releaseLocked(position)
}

func (desk *Desk) releaseLocked(position *Position) error {
	if position == nil || position.Symbol == "" {
		return nil
	}

	cache, key := desk.cacheFor(position.Virtual)

	if !cache.Has(position.Symbol) {
		return nil
	}

	if position.Virtual {
		types.SetVirtualOpen(position.Symbol, false)
		desk.archiveVirtual(position)
		desk.virtuals.Remove(position.Symbol)
		desk.pendingTrain.Store(position.Symbol, position)
	} else {
		desk.positions.Remove(position.Symbol)
	}

	value := data.NewValue(
		key, "closed", position.Symbol,
	).Append(
		position, "position",
	)

	if err := desk.history.Move(
		value, key+"/open/"+position.Symbol+"/",
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Exit] failed to store position",
			err,
		))
	}

	desk.publishChanged([]*Position{position})

	return nil
}

/*
Drop retires a virtual from the desk entirely, open or closed.
*/
func (desk *Desk) Drop(symbol string) {
	types.SetVirtualOpen(symbol, false)
	desk.virtuals.Drop(symbol)
}
