package broker

import (
	"context"
	"errors"
	"iter"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/pipeline"
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
	live      bool
	onTrade   func(*market.Trade)
	level3Ch  chan *market.Level3
	tradeCh   chan *market.Trade
	l3Symbols []string
}

/*
NewDesk picks tier wiring from market.model so callers do not duplicate the
live-only vs virtual+paper+disarmed-real matrix at every boot site. It also
seeds price.balance so paper tiers size from the same ledger Enter spends.
*/
func NewDesk(
	ctx context.Context,
	transports map[string]network.Client,
	price *Price,
	client *store.Client,
) *Desk {
	quote := system.Cfg.Market.QuoteCurrency
	live := system.Cfg.Market.Model == "real"

	if price != nil {
		price.balance = market.NewBalance(map[string]any{
			"quote":  quote,
			"assets": []*market.Asset{},
		})
	}

	var names *spot.Normalizer

	if price != nil {
		names = price.normalizer
	}

	host := venue.NewKraken(quote).Use(names)
	tiers := make(map[TierType]*Tier)

	if live {
		// Live book: positions and balances come from the exchange only.
		real := NewTier(
			ctx,
			TierTypeReal,
			transports["public"],
			transports["private"],
			price,
			venue.NewKraken(quote).Use(names),
			nil,
		)
		tiers[TierTypeReal] = real
	} else {
		virtual := NewTier(
			ctx,
			TierTypeVirtual,
			transports["public"],
			transports["virtual"],
			price,
			venue.NewKraken(quote).Use(names),
			client,
		)
		paper := NewTier(
			ctx,
			TierTypePaper,
			transports["public"],
			transports["paper"],
			price,
			venue.NewKraken(quote).Use(names),
			client,
		)
		real := NewTier(
			ctx,
			TierTypeReal,
			transports["public"],
			transports["private"],
			price,
			venue.NewKraken(quote).Use(names),
			client,
		)
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
		live:     live,
	}

	go desk.pumpLevel3()
	go desk.pumpTrade()

	desk.publishModes()
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
			trades := make([]*market.TradeData, 0)

			for _, row := range book.Data {
				if row != nil && row.Symbol != "" {
					desk.l3Symbols = append(desk.l3Symbols, row.Symbol)

					trades = append(trades, &market.TradeData{
						Symbol: row.Symbol,
					})
				}
			}

			desk.Update(trades)
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

		pipeline.Hit("trade")
		desk.ingest(desk.venue.Read(buf))
	}
}

/*
ingest applies one decoded public frame. venue.Read only demuxes.
*/
func (desk *Desk) ingest(frame market.MarketType) {
	if frame == nil {
		return
	}

	switch typed := frame.(type) {
	case *kraken.Trade:
		trade := market.NewTrade("", "", nil)
		typed.ToMarket(trade)
		desk.enqueueTrade(trade)
	}
}

/*
Hello is the operator-facing book: balance changes and history sync call it so
the hub stays consistent without every tier mutating UI directly.
*/
func (desk *Desk) Hello() {
	desk.publishModes()

	for _, tier := range desk.tiers {
		tier.Hello()
	}
}

/*
publishModes tells the hub which books this process actually runs so the
mode strip does not hardcode virtual/paper as live.
*/
func (desk *Desk) publishModes() {
	virtual := desk.tiers[TierTypeVirtual] != nil
	paper := desk.tiers[TierTypePaper] != nil
	real := desk.tiers[TierTypeReal] != nil

	book := func(active, armed bool) map[string]any {
		return map[string]any{
			"active": active,
			"armed":  armed,
			"halted": active && !armed,
		}
	}

	ui.UI.WriteJSON(map[string]any{
		"confidence": map[string]any{
			"virtual":             book(virtual, virtual && !desk.live),
			"paper":               book(paper, paper && !desk.live),
			"real":                book(real, desk.live && real),
			"eligible_resistance": system.Cfg.Learning.PaperEligibleResistance,
			"real_ceiling":        desk.live && real,
		},
	})
}

/*
Flush persists every open position (including Closing) before process exit.
*/
func (desk *Desk) Flush() error {
	var errs error

	for _, tier := range desk.tiers {
		if tier == nil || tier.positions == nil {
			continue
		}

		for _, position := range tier.positions.Open() {
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
Update records tape flow and marks open positions from the executable bid.
Level3 and public trades both land here — after the batch is marked, Hello
ships one full open-universe positions_snapshot so the chart consolidates
every tick, not only when the touched symbol already held a READY row.
*/
func (desk *Desk) Update(trades []*market.TradeData) {
	for _, trade := range trades {
		if trade == nil {
			continue
		}

		desk.price.NoteTrade(trade)
		symbol := trade.Symbol
		symbol = desk.price.normalizer.Name(symbol)
		mark := desk.price.Mark(symbol)

		if mark == nil {
			continue
		}

		for _, tier := range desk.tiers {
			if tier == nil {
				continue
			}

			tier.Update(symbol, mark)
		}
	}

	desk.Hello()
}

/*
OpenCount is how many live positions a tier holds — board capacity uses this.
*/
func (desk *Desk) OpenCount(tt TierType) int {
	tier := desk.tiers[tt]

	if tier == nil || tier.positions == nil {
		return 0
	}

	return len(tier.positions.Open())
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
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"[desk.Enter] unknown tier",
			nil,
		))
	}

	return errnie.Error(tier.Enter(symbol, path, pathFound))
}

/*
Exit routes policy exits through the tier that holds the cache row so Send uses
the correct private socket and fill matching stays on that lane's positions.
*/
func (desk *Desk) Exit(
	tt TierType, position *Position, force bool,
) error {
	tier := desk.tiers[tt]

	if tier == nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"[desk.Exit] unknown tier",
			nil,
		))
	}

	return errnie.Error(tier.Exit(position, force))
}

/*
GetPosition returns the position by tier and symbol.
*/
func (desk *Desk) GetPosition(tt TierType, symbol string) *Position {
	tier := desk.tiers[tt]

	if tier == nil {
		return nil
	}

	return tier.positions.Get(symbol)
}

/*
Has is the fast guard inside mark/Refresh — multi-tier paper+real may share
symbols but only tiers with open positions need Update work.
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
Positions exposes filled opens to strategy without Submitted noise from pending
enters still awaiting venue buy prints.
*/
func (desk *Desk) Positions(tt TierType, symbol string) iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		tier := desk.tiers[tt]

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
learning still needs the position handle for attribution before ClearPending.
*/
func (desk *Desk) Pending(tt TierType) iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		tier := desk.tiers[tt]

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
func (desk *Desk) ClearPending(tt TierType, symbol string) {
	if tier := desk.tiers[tt]; tier != nil {
		tier.ClearPending(symbol)
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
