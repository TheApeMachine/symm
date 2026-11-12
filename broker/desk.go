package broker

import (
	"context"
	"errors"
	"iter"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
tradeQueueFrames caps trade latency: full ring drops oldest prints so pumpTrade
never blocks the public reader; L3 instead blocks because gaps corrupt books.
*/
const tradeQueueFrames = 256

/*
Desk fans work across the armed tiers. Live-only (market.model real) builds
the real book alone; paper mode keeps virtual + paper + disarmed real.
*/
type Desk struct {
	*runtime.System
	price     *Price
	public    network.Client
	venue     market.Venue
	tiers     map[TierType]*Tier
	onTrade   func(*market.Trade)
	level3Ch  chan *market.Level3
	tradeCh   chan *market.Trade
	l3Symbols []string
}

/*
NewDesk picks tier wiring from market.model so callers do not duplicate the
live-only vs virtual+paper+disarmed-real matrix at every boot site.
*/
func NewDesk(
	ctx context.Context,
	transports map[string]network.Client,
	price *Price,
	client *store.Client,
) *Desk {
	return DeskFor(ctx, transports, price, client, system.Cfg.Market.Model == "real")
}

/*
DeskFor is the explicit market.mode hook tests use; it also seeds price.balance
so paper tiers size from the same ledger object Enter spends.
*/
func DeskFor(
	ctx context.Context,
	transports map[string]network.Client,
	price *Price,
	client *store.Client,
	live bool,
) *Desk {
	quote := system.Cfg.Market.QuoteCurrency

	if price != nil {
		price.balance = market.NewBalance(map[string]any{
			"quote":  quote,
			"assets": []*market.Asset{},
		})
	}

	host := venue.NewKraken(quote)
	tiers := make(map[TierType]*Tier)

	if live {
		// Live book: positions and balances come from the exchange only.
		real := NewTier(
			ctx,
			TierTypeReal,
			transports["public"],
			transports["private"],
			price,
			venue.NewKraken(quote),
			nil,
		)
		real.live = true
		tiers[TierTypeReal] = real
	} else {
		virtual := NewTier(
			ctx,
			TierTypeVirtual,
			transports["public"],
			transports["virtual"],
			price,
			venue.NewKraken(quote),
			client,
		)
		virtual.live = false
		paper := NewTier(
			ctx,
			TierTypePaper,
			transports["public"],
			transports["paper"],
			price,
			venue.NewKraken(quote),
			client,
		)
		paper.live = false
		real := NewTier(
			ctx,
			TierTypeReal,
			transports["public"],
			transports["private"],
			price,
			venue.NewKraken(quote),
			client,
		)
		real.live = false
		tiers[TierTypeVirtual] = virtual
		tiers[TierTypePaper] = paper
		tiers[TierTypeReal] = real
	}

	desk := &Desk{
		System:   runtime.NewSystem(ctx, "desk"),
		price:    price,
		public:   transports["public"],
		venue:    host,
		level3Ch: make(chan *market.Level3, system.Cfg.Market.Level3.RingCapacity),
		tradeCh:  make(chan *market.Trade, tradeQueueFrames),
		tiers:    tiers,
	}

	go desk.pumpLevel3()
	go desk.pumpTrade()

	host.Trade().OnChange(func(frame market.MarketType) {
		trade := market.NewTrade("", "", nil)
		frame.ToMarket(trade)
		desk.enqueueTrade(trade)
	})

	desk.Transition(runtime.READY)
	return desk
}

/*
enqueueLevel3 applies backpressure instead of dropping deltas. A dropped
Level3 frame permanently diverges the book until the next snapshot.
*/
func (desk *Desk) enqueueLevel3(book *market.Level3) {
	if book == nil {
		return
	}

	select {
	case desk.level3Ch <- book:
	case <-desk.Context().Done():
	}
}

/*
pumpLevel3 serializes book apply and Refresh so concurrent L3 batches do not
reorder deltas and leave crossed or checksum-diverged state mid-frame.
*/
func (desk *Desk) pumpLevel3() {
	for {
		select {
		case <-desk.Context().Done():
			return
		case book := <-desk.level3Ch:
			if desk.price != nil && desk.price.Books != nil {
				desk.price.Books.Update(book)
			}

			desk.l3Symbols = desk.l3Symbols[:0]

			for _, row := range book.Data {
				if row != nil && row.Symbol != "" {
					desk.l3Symbols = append(desk.l3Symbols, row.Symbol)
				}
			}

			desk.Refresh(desk.l3Symbols)
		}
	}
}

/*
enqueueTrade pushes a trade snapshot without blocking. Full ring drops oldest.
*/
func (desk *Desk) enqueueTrade(trade *market.Trade) {
	if trade == nil {
		return
	}

	select {
	case desk.tradeCh <- trade:
		return
	default:
	}

	select {
	case <-desk.tradeCh:
	default:
	}

	select {
	case desk.tradeCh <- trade:
	default:
	}
}

/*
pumpTrade keeps tape uptake and strategy ticks off the public Read thread so
slow marks cannot stall websocket ingress and drop trades.
*/
func (desk *Desk) pumpTrade() {
	for {
		select {
		case <-desk.Context().Done():
			return
		case trade := <-desk.tradeCh:
			desk.Update(trade.Data)

			if desk.onTrade != nil {
				desk.onTrade(trade)
			}
		}
	}
}

/*
OnTrade decouples signals from desk internals — cmd wires the workspace consumer
without desk importing strategy packages.
*/
func (desk *Desk) OnTrade(fn func(*market.Trade)) {
	desk.onTrade = fn
}

/*
Feed decodes one Level3 socket frame into a private book copy and enqueues it.
Each L3 batch has its own reader; sharing a venue Level3 would race.
*/
func (desk *Desk) Feed(buf []byte) {
	var head struct {
		Channel string `json:"channel"`
	}

	if err := sonic.Unmarshal(buf, &head); err != nil || head.Channel != "level3" {
		return
	}

	raw := kraken.DecodeLevel3(buf)
	if raw == nil || len(raw.Data) == 0 {
		return
	}

	book := market.NewLevel3()
	raw.ToMarket(book)
	desk.enqueueLevel3(book)
}

/*
OpenPrivate is separated from Run so boot can finish instrument/L3 before
executions and add_order share one authenticated private socket.
*/
func (desk *Desk) OpenPrivate() error {
	for _, tier := range desk.tiers {
		if tier == nil || tier.Type != TierTypeReal {
			continue
		}

		if err := tier.OpenPrivate(); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

/*
Start launches Run in the background after OpenPrivate wired sockets — boot
calls Flush/Hello synchronously before ingress goroutines race fills.
*/
func (desk *Desk) Start() {
	go desk.Run()
}

/*
Run multiplexes one public Read into the shared venue decoder; each tier still
owns its private Read loop for executions and balances.
*/
func (desk *Desk) Run() {
	for _, tier := range desk.tiers {
		if tier == nil {
			continue
		}

		go tier.Run()
	}

	if desk.public == nil || desk.venue == nil {
		return
	}

	for {
		select {
		case <-desk.Context().Done():
			return
		default:
		}

		buf, err := desk.public.Read()

		if err != nil {
			if desk.Context().Err() != nil {
				return
			}

			if !network.Recoverable(err) {
				errnie.Error(errnie.Err(
					errnie.IO, "[desk] public ingress read failed", err,
				))
			}

			continue
		}

		desk.venue.Read(buf)
	}
}

/*
Hello is the operator-facing book: balance changes and history sync call it so
the hub stays consistent without every tier mutating UI directly.
*/
func (desk *Desk) Hello() {
	ui.UI.WriteJSON(desk.helloFrame())
}

/*
helloFrame recomputes marks before JSON so UI lines match what kickExit sees;
it also merges exchangeClosed trips absent from the tier cache.
*/
func (desk *Desk) helloFrame() map[string]any {
	positions := make(map[string]*Position)
	balances := make(map[string]any)

	if real := desk.tiers[TierTypeReal]; real != nil {
		real.Unlocked(desk.tiers[TierTypePaper])
	}

	for _, tier := range desk.tiers {
		if tier == nil {
			continue
		}

		for _, position := range tier.positions.All() {
			if position == nil || position.ID == "" {
				continue
			}

			positions[position.ID] = position
		}

		if tier.Type == TierTypeReal {
			tier.exchangeClosed.Range(func(_, value any) bool {
				position := value.(*Position)

				if position == nil || position.ID == "" {
					return true
				}

				positions[position.ID] = position

				return true
			})
		}

		if reading := tier.balanceReading(); reading != nil {
			balances[tier.Type.String()] = reading
		}
	}

	return map[string]any{
		"positions": positions,
		"balances":  balances,
	}
}

/*
Flush persists every open lot (including Closing) before process exit.
*/
func (desk *Desk) Flush() error {
	var errs error

	for _, tier := range desk.tiers {
		if tier == nil || tier.positions == nil {
			continue
		}

		for _, position := range tier.positions.Submitted() {
			if position == nil {
				continue
			}

			if err := tier.positions.Save(position); err != nil {
				errs = errors.Join(errs, err)
			}
		}
	}

	return errs
}

/*
Update records tape flow and marks open lots from the executable bid, not the print.
*/
func (desk *Desk) Update(trades []*market.TradeData) {
	for _, trade := range trades {
		if trade == nil {
			continue
		}

		if desk.price == nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[desk.Update] price is not wired",
				nil,
			), "symbol", trade.Symbol)
			continue
		}

		desk.price.NoteTrade(trade)

		mark := desk.price.TouchBid(trade.Symbol)

		if mark == nil && desk.Has(trade.Symbol) {
			mark = desk.price.CloseMark(trade.Symbol)
		}

		if mark == nil {
			continue
		}

		desk.mark(trade.Symbol, mark)
	}
}

/*
Refresh marks on bid moves without waiting for a trade print — otherwise stoploss
would lag when the book reprices in a quiet market.
*/
func (desk *Desk) Refresh(symbols []string) {
	for _, symbol := range symbols {
		if symbol == "" || desk.price == nil {
			continue
		}

		price := desk.price.TouchBid(symbol)

		if price == nil && desk.Has(symbol) {
			price = desk.price.CloseMark(symbol)
		}

		if price != nil {
			desk.mark(symbol, price)
		}
	}
}

/*
mark skips idle symbols so pumpLevel3 does not publish ledger noise for the
entire subscribed universe on every L3 delta.
*/
func (desk *Desk) mark(symbol string, price *decimal.Decimal) {
	if symbol == "" || price == nil {
		return
	}

	// L3 frames touch the whole universe; skip tiers with no open lot so
	// pumpLevel3 does not thrash ledger/hub for idle symbols.
	if !desk.Has(symbol) {
		return
	}

	update := market.NewUpdate(symbol, price)

	for _, tier := range desk.tiers {
		if tier == nil {
			continue
		}

		tier.Update(update)
	}
}

/*
Apply must touch every tier because paper and real sockets differ but symbol
keys collide — one execution frame might belong to any armed lane's submit.
*/
func (desk *Desk) Apply(executions *market.Execution) {
	if executions == nil {
		return
	}

	for _, tier := range desk.tiers {
		if tier == nil {
			continue
		}

		tier.apply(executions)
	}
}

/*
Enter is the desk entrypoint so strategy does not pick transports or Unlocked
logic — wrong tier wiring would send real orders without paper proof.
*/
func (desk *Desk) Enter(
	symbol string, path string, tt TierType, pathFound bool,
) error {
	tier := desk.tiers[tt]

	if tier == nil {
		return errnie.Err(
			errnie.NotFound,
			"[desk.Enter] unknown tier",
			nil,
		)
	}

	tier.Unlocked(desk.tiers[TierTypePaper])
	return tier.Enter(symbol, path, pathFound)
}

/*
Exit routes policy exits through the tier that holds the cache row so Send uses
the correct private socket and fill matching stays on that lane's positions.
*/
func (desk *Desk) Exit(
	tier TierType, position *Position,
) error {
	t := desk.tiers[tier]

	if t == nil {
		return errnie.Err(
			errnie.NotFound,
			"[desk.Exit] unknown tier",
			nil,
		)
	}

	return t.Exit(position)
}

/*
Has is the fast guard inside mark/Refresh — multi-tier paper+real may share
symbols but only tiers with open lots need Update work.
*/
func (desk *Desk) Has(symbol string) bool {
	for _, tier := range desk.tiers {
		if tier == nil || tier.positions == nil {
			continue
		}

		if tier.positions.Has(symbol) {
			return true
		}
	}

	return false
}

/*
OpenCount is the number of non-flat lots on one tier — board capacity uses this.
*/
func (desk *Desk) OpenCount(tt TierType) int {
	tier := desk.tiers[tt]

	if tier == nil || tier.positions == nil {
		return 0
	}

	return tier.positions.OpenCount()
}

/*
ChainOnSettle fans settle events to an extra listener on every armed tier.
*/
func (desk *Desk) ChainOnSettle(fn func(*Position)) {
	if desk == nil || fn == nil {
		return
	}

	for _, tier := range desk.tiers {
		if tier == nil || tier.positions == nil {
			continue
		}

		tier.positions.ChainOnSettle(fn)
	}
}

/*
AwaitingFill lets strategy defer path learning until Apply owns qty — acting on
a submitted-but-unfilled lot would attribute fills to the wrong policy epoch.
*/
func (desk *Desk) AwaitingFill(symbol string) bool {
	if symbol == "" {
		return false
	}

	for _, tier := range desk.tiers {
		if tier == nil || tier.positions == nil {
			continue
		}

		for _, position := range tier.positions.Submitted(symbol) {
			if position == nil {
				continue
			}

			if position.Qty == nil || position.Qty.Sign() <= 0 {
				return true
			}
		}
	}

	return false
}

/*
Positions exposes filled opens to strategy without Submitted noise from pending
enters still awaiting venue buy prints.
*/
func (desk *Desk) Positions(tier TierType, symbol string) iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		tier := desk.tiers[tier]

		if tier == nil {
			return
		}

		for _, position := range tier.positions.Open(symbol) {
			if !yield(position) {
				return
			}
		}
	}
}

/*
Pending holds flat sells until the model records exit_path — Remove ran but
learning still needs the lot handle for attribution before ClearPending.
*/
func (desk *Desk) Pending(tier TierType) iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		tier := desk.tiers[tier]

		if tier == nil {
			return
		}

		for position := range tier.Pending() {
			if !yield(position) {
				return
			}
		}
	}
}

/*
ClearPending frees pending map memory and lets the symbol re-enter after the
model finished writing exit_path into its tables.
*/
func (desk *Desk) ClearPending(tier TierType, symbol string) {
	if t := desk.tiers[tier]; t != nil {
		t.ClearPending(symbol)
	}
}

/*
CanSize is the cheap pre-check before model work — it reuses Allocate so Observe
and enter share one sizing truth.
*/
func (desk *Desk) CanSize(symbol string) bool {
	if symbol == "" || desk.price == nil {
		return false
	}

	qty, _ := desk.price.Allocate(symbol, nil)
	return qty != nil && qty.Sign() > 0
}

/*
BookStale surfaces instrument disconnect to strategy before Observe spends CPU
on a book that will never pass BookIntact.
*/
func (desk *Desk) BookStale(symbol string) bool {
	if desk.price == nil || desk.price.Books == nil {
		return true
	}

	return desk.price.Books.StaleSymbol(symbol)
}

/*
PaperBook stands up a deterministic L3 snapshot and three tiers without Kraken
so broker tests exercise fill paths without credentials or ws replay.
*/
func PaperBook(ctx context.Context, symbol string, transport network.Client) (*Desk, *Price) {
	qtyMin := decimal.RequireFromString("0.01")
	step := decimal.RequireFromString("0.01")
	costMin := decimal.RequireFromString("1")
	fee := decimal.NewFromFloat(0.26)

	base, quote, _ := strings.Cut(symbol, "/")
	if quote == "" {
		quote = "USD"
	}

	instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}, quote: quote}
	instrument.cache.Store(symbol, kraken.InstrumentPair{
		Symbol: symbol, Base: base, Quote: quote, Status: "online",
		QtyMin: &qtyMin, CostMin: &costMin, QtyIncrement: &step, TickSize: &step,
	})
	instrument.takerFees.Store(symbol, &fee)

	books := NewBookManager()
	bidPx := decimal.NewFromInt(100)
	bidQty := decimal.NewFromInt(50)
	askPx := decimal.NewFromInt(101)
	askQty := decimal.NewFromInt(50)

	books.Update(&market.Level3{
		Type: "snapshot",
		Data: []*market.Level3Data{{
			Symbol: symbol,
			Bids: []*market.Level3Order{{
				LimitPrice: &bidPx,
				OrderQty:   &bidQty,
			}},
			Asks: []*market.Level3Order{{
				LimitPrice: &askPx,
				OrderQty:   &askQty,
			}},
		}},
	})

	cash := decimal.NewFromInt(100_000)
	wallet := market.NewBalance(map[string]any{
		"quote": "USD",
		"assets": []*market.Asset{
			market.NewAsset(nil, "USD", cash, decimal.Zero, cash),
		},
	})

	host := venue.NewKraken(quote)
	host.Balance().(*kraken.Balance).Result = map[string]*decimal.Decimal{quote: &cash}

	price := &Price{
		Instrument: instrument,
		Books:      books,
		ticker:     market.NewTicker(nil),
		tape:       NewTape(),
		balance:    wallet,
		normalizer: NormalizerFor(instrument),
		venue:      host,
	}
	price.bindRisk()

	desk := AssembledDesk(price, map[TierType]*Tier{
		TierTypeVirtual: AssembledTier(ctx, TierTypeVirtual, price, host, transport),
		TierTypePaper:   AssembledTier(ctx, TierTypePaper, price, host, transport),
		TierTypeReal:    AssembledTier(ctx, TierTypeReal, price, host, transport),
	})

	return desk, price
}

/*
PaperDesk drops the Price return when tests only assert routing, not sizing math.
*/
func PaperDesk(ctx context.Context, symbol string, transport network.Client) *Desk {
	desk, _ := PaperBook(ctx, symbol, transport)

	return desk
}

/*
AssembledDesk wires tiers for cross-package tests without live transports.
*/
func AssembledDesk(price *Price, tiers map[TierType]*Tier) *Desk {
	return &Desk{price: price, tiers: tiers}
}
