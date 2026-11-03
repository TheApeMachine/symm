package broker

import (
	"context"
	"iter"

	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/venue"
)

// Drop-oldest ring depths for Level3 / trade pumps (same shape as ui.Hub).
const (
	level3QueueFrames = 256
	tradeQueueFrames  = 256
)

/*
Desk fans work across the virtual, paper, and real tiers.
*/
type Desk struct {
	*runtime.System
	price    *Price
	public   network.Client
	venue    market.Venue
	tiers    map[TierType]*Tier
	onTrade  func(*market.Trade)
	level3Ch chan *market.Level3
	tradeCh  chan *market.Trade
}

func NewDesk(
	ctx context.Context,
	transports map[string]network.Client,
	price *Price,
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
		level3Ch: make(chan *market.Level3, level3QueueFrames),
		tradeCh:  make(chan *market.Trade, tradeQueueFrames),
		tiers: map[TierType]*Tier{
			TierTypeVirtual: NewTier(
				ctx,
				TierTypeVirtual,
				transports["public"],
				transports["virtual"],
				price,
				venue.NewKraken(quote),
			),
			TierTypePaper: NewTier(
				ctx,
				TierTypePaper,
				transports["public"],
				transports["paper"],
				price,
				venue.NewKraken(quote),
			),
			TierTypeReal: NewTier(
				ctx,
				TierTypeReal,
				transports["public"],
				transports["private"],
				price,
				venue.NewKraken(quote),
			),
		},
	}

	go desk.pumpLevel3()
	go desk.pumpTrade()

	host.Level3().OnChange(func(frame market.MarketType) {
		book := market.NewLevel3()
		frame.ToMarket(book)
		desk.enqueueLevel3(book)
	})

	host.Trade().OnChange(func(frame market.MarketType) {
		trade := market.NewTrade("", "", nil)
		frame.ToMarket(trade)
		desk.enqueueTrade(trade)
	})

	desk.Transition(runtime.READY)
	return desk
}

/*
enqueueLevel3 pushes a book snapshot without blocking. A full ring drops the
oldest frame so the newest always lands — same shape as ui.Hub.Write.
*/
func (desk *Desk) enqueueLevel3(book *market.Level3) {
	if book == nil {
		return
	}

	select {
	case desk.level3Ch <- book:
		return
	default:
	}

	select {
	case <-desk.level3Ch:
	default:
	}

	select {
	case desk.level3Ch <- book:
	default:
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

			symbols := make([]string, 0, len(book.Data))

			for _, row := range book.Data {
				if row != nil && row.Symbol != "" {
					symbols = append(symbols, row.Symbol)
				}
			}

			desk.Refresh(symbols)
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
Feed pushes a raw venue frame through the desk venue (level3 sockets use this).
*/
func (desk *Desk) Feed(buf []byte) {
	if desk.venue == nil {
		return
	}

	desk.venue.Read(buf)
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
				position.Update(nil)
			}

			positions[position.ID] = position
		}

		balances[tier.Type.String()] = tier.walletReading()
	}

	ui.UI.WriteJSON(map[string]any{
		"positions_mode": "snapshot",
		"positions":      positions,
		"balances":       balances,
	})
}

func (desk *Desk) Update(trades []*market.TradeData) {
	for _, trade := range trades {
		if trade == nil {
			continue
		}

		price := trade.Price
		desk.mark(trade.Symbol, &price)
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
