package broker

import (
	"context"
	"errors"
	"iter"
	"time"

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

// Trade pump is drop-oldest; Level3 uses configured backpressure capacity.
const tradeQueueFrames = 256

/*
Desk fans work across the virtual, paper, and real tiers.
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

func NewDesk(
	ctx context.Context,
	transports map[string]network.Client,
	price *Price,
	client *store.Client,
) *Desk {
	quote := system.Cfg.Market.QuoteCurrency

	if price != nil {
		price.balance = market.NewBalance(map[string]any{
			"quote":  quote,
			"assets": []*market.Asset{},
		})
	}

	host := venue.NewKraken(quote)

	desk := &Desk{
		System:   runtime.NewSystem(ctx, "desk"),
		price:    price,
		public:   transports["public"],
		venue:    host,
		level3Ch: make(chan *market.Level3, system.Cfg.Market.Level3.RingCapacity),
		tradeCh:  make(chan *market.Trade, tradeQueueFrames),
		tiers: map[TierType]*Tier{
			TierTypeVirtual: NewTier(
				ctx,
				TierTypeVirtual,
				transports["public"],
				transports["virtual"],
				price,
				venue.NewKraken(quote),
				client,
			),
			TierTypePaper: NewTier(
				ctx,
				TierTypePaper,
				transports["public"],
				transports["paper"],
				price,
				venue.NewKraken(quote),
				client,
			),
			TierTypeReal: NewTier(
				ctx,
				TierTypeReal,
				transports["public"],
				transports["private"],
				price,
				venue.NewKraken(quote),
				client,
			),
		},
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
pumpLevel3 applies queued Level3 snapshots in order on one worker.
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
pumpTrade marks lots and fans the workspace tick from queued trades in order.
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
OnTrade registers the workspace tick sink for public trade frames.
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
Start begins per-tier private ingress and the shared public reader.
*/
func (desk *Desk) Start() {
	go desk.Run()
}

/*
Run starts per-tier private ingress and the shared public reader.
*/
func (desk *Desk) Run() {
	for _, tier := range desk.tiers {
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

			continue
		}

		desk.venue.Read(buf)
	}
}

func (desk *Desk) Hello() {
	positions := make(map[string]*Position)
	balances := make(map[string]any)

	for _, tier := range desk.tiers {
		if tier == nil {
			continue
		}

		for _, position := range tier.positions.All() {
			if position == nil || position.ID == "" {
				continue
			}

			if position.ClosedAt == nil {
				mark := position.Mark
				if mark == nil && desk.price != nil {
					mark = desk.price.TouchBid(position.Symbol)
				}
				if err := position.Update(mark); err != nil {
					errnie.Error(err)
				}
			}

			positions[position.ID] = position
		}

		balances[tier.Type.String()] = tier.wallet.Reading()
	}

	ui.UI.WriteJSON(map[string]any{
		"positions_mode": "snapshot",
		"positions":      positions,
		"balances":       balances,
	})
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

func (desk *Desk) Update(trades []*market.TradeData) {
	for _, trade := range trades {
		if trade == nil {
			continue
		}

		if desk.price != nil {
			at := trade.Timestamp

			if at.IsZero() {
				at = time.Now()
			}

			quote := trade.Price.Mul(trade.Qty).InexactFloat64()
			desk.price.tape.Note(trade.Symbol, quote, at)
		}

		// Longs mark on the executable bid, never the print. Aggressive
		// buys print the ask and would inflate Peak before an L3 bid mark.
		var price *decimal.Decimal

		if desk.price != nil && desk.price.Books != nil {
			desk.price.Books.Book(trade.Symbol, func(managed *market.Book) {
				if managed == nil || managed.BestBid() == nil || managed.BestBid().Price == nil {
					return
				}

				price = managed.BestBid().Price
			})
		}

		if price == nil {
			p := trade.Price
			price = &p
		}

		desk.mark(trade.Symbol, price)
	}
}

func (desk *Desk) Refresh(symbols []string) {
	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		var price *decimal.Decimal

		desk.price.Books.Book(symbol, func(managed *market.Book) {
			if managed == nil {
				return
			}

			bid := managed.BestBid()
			if bid == nil || bid.Price == nil {
				return
			}

			price = bid.Price
		})

		if price != nil {
			desk.mark(symbol, price)
		}
	}
}

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
		tier.Update(update)
	}
}

func (desk *Desk) Apply(executions *market.Execution) {
	if executions == nil {
		return
	}

	for _, tier := range desk.tiers {
		tier.apply(executions)
	}
}

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

func (desk *Desk) Exit(
	tier TierType, position *Position,
) error {
	return desk.tiers[tier].Exit(position)
}

func (desk *Desk) Has(symbol string) bool {
	for _, tier := range desk.tiers {
		if tier.positions.Has(symbol) {
			return true
		}
	}

	return false
}

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
Pending yields stoploss-settled lots waiting for the model to record an exit path.
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
ClearPending drops a settled lot after the model has attributed its exit path.
*/
func (desk *Desk) ClearPending(tier TierType, symbol string) {
	if t := desk.tiers[tier]; t != nil {
		t.ClearPending(symbol)
	}
}

func (desk *Desk) CanSize(symbol string) bool {
	if symbol == "" || desk.price == nil {
		return false
	}

	qty, _ := desk.price.Allocate(symbol, nil)
	return qty != nil && qty.Sign() > 0
}

func (desk *Desk) BookStale(symbol string) bool {
	if desk.price == nil || desk.price.Books == nil {
		return true
	}

	return desk.price.Books.StaleSymbol(symbol)
}
