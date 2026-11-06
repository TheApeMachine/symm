package broker

import (
	"context"
	"encoding/json"
	"iter"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

type TierType uint8

const (
	TierTypeVirtual TierType = iota
	TierTypePaper
	TierTypeReal
)

func (tt TierType) String() string {
	switch tt {
	case TierTypeVirtual:
		return "virtual"
	case TierTypePaper:
		return "paper"
	case TierTypeReal:
		return "real"
	}

	errnie.Error(errnie.Err(
		errnie.NotFound,
		"[broker.TierType] invalid tier type",
		nil,
	))

	return ""
}

type Tier struct {
	ctx       context.Context
	Type      TierType
	WinRate   float64
	Edge      float64
	public    network.Client
	private   network.Client
	price     *Price
	venue     market.Venue
	wallet    *Wallet
	positions *Cache
	pending   *sync.Map
	inflight  sync.Map
	savedAt   sync.Map
	reqSeq    atomic.Int64
	live      *Live
	unlocked  bool
	halted    bool
	sawOK     bool
	broke     bool
	reprieve  bool
}

/*
submission is one add_order waiting on a venue ack, keyed by req_id.
*/
type submission struct {
	position *Position
	action   string
	reqID    int64
}

func NewTier(
	ctx context.Context,
	tt TierType,
	public network.Client,
	private network.Client,
	price *Price,
	host market.Venue,
	client *store.Client,
) *Tier {
	// Real orders leave the process only when market.model is real.
	if tt == TierTypeReal && system.Cfg.Market.Model != "real" {
		private = nil
	}

	tier := &Tier{
		ctx:     ctx,
		Type:    tt,
		public:  public,
		private: private,
		price:   price,
		venue:   host,
		pending: &sync.Map{},
	}

	if tt == TierTypeReal && private != nil {
		tier.live = NewLive(ctx, private)

		if tier.live == nil {
			tier.private = nil
			private = nil
		}
	}

	tier.positions = NewCache(
		ctx, tt.String(), private, price, tier.settle, client,
	)
	tier.wallet = NewWallet(host)

	// Virtual wallet is process-local; reopen credits base for S3 open lots
	// before the balance prime so settle can sell.
	if tt == TierTypeVirtual {
		if virtual, ok := private.(*network.Virtual); ok {
			for _, position := range tier.positions.Open() {
				position.Restock(virtual)
			}
		}
	}

	if host != nil {
		host.Balance().OnChange(func(frame market.MarketType) {
			if tier.Type == TierTypePaper && tier.price != nil && tier.price.balance != nil {
				frame.ToMarket(tier.price.balance)
			}

			tier.Hello()
		})

		host.TradeBalance().OnChange(func(market.MarketType) {
			tier.Hello()
		})

		host.Execution().OnChange(func(frame market.MarketType) {
			book := &market.Execution{}
			frame.ToMarket(book)
			tier.apply(book)
		})

		if book, ok := host.(*venue.Kraken); ok && book.Order() != nil {
			book.Order().OnChange(func(frame market.MarketType) {
				order, ok := frame.(*kraken.Order)

				if !ok || order == nil {
					return
				}

				tier.ack(order)
			})
		}
	}

	if tier.live != nil {
		if err := OpenWS(tier.private, system.Cfg.WebSocket.Endpoints.Private); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[broker.Tier] private websocket open failed", err,
			))
		}
	}

	// Virtual and paper serve the balance paths themselves. Real signs the
	// same REST calls and also folds the private balances channel.
	if host != nil && tier.private != nil &&
		(tier.Type == TierTypeVirtual || tier.Type == TierTypePaper || tier.live != nil) {
		balanceURL := system.Cfg.WebSocket.Endpoints.Balance
		tradeBalanceURL := system.Cfg.WebSocket.Endpoints.TradeBalance

		if tier.live != nil {
			balanceURL = tier.live.BalanceURL()
			tradeBalanceURL = tier.live.TradeBalanceURL()
		}

		tier.private.Watch(
			balanceURL,
			host.Balance().Request(),
			host.Balance(),
		)
		tier.private.Watch(
			tradeBalanceURL,
			host.TradeBalance().Request(),
			host.TradeBalance(),
		)

		// Prime once so Allocate / wallet see cash before the first Watch tick.
		if raw, err := tier.private.Post(
			balanceURL,
			host.Balance().Request(),
		); err == nil {
			_ = json.Unmarshal(raw, host.Balance())
		}

		if raw, err := tier.private.Post(
			tradeBalanceURL,
			host.TradeBalance().Request(),
		); err == nil {
			_ = json.Unmarshal(raw, host.TradeBalance())
		}
	}

	tier.Recalculate()
	tier.unlocked = tier.Unlocked(nil)

	return tier
}

func (tier *Tier) apply(executions *market.Execution) {
	if executions == nil {
		return
	}

	touched := make(map[string]*Position)

	for _, execution := range executions.Data {
		if execution == nil || !tier.fillReport(execution) {
			continue
		}

		matched := false

		for _, position := range tier.positions.Submitted(execution.Symbol) {
			if position == nil || !position.Owns(*execution) {
				continue
			}

			position.Apply(*execution)
			touched[position.ID] = position
			matched = true
		}

		if matched {
			continue
		}

		for _, position := range tier.positions.Closed(execution.Symbol) {
			if position == nil || !position.Owns(*execution) {
				continue
			}

			position.Apply(*execution)
			touched[position.ID] = position
		}
	}

	if len(touched) == 0 {
		return
	}

	for _, position := range touched {
		if err := tier.positions.Save(position); err != nil {
			errnie.Error(err)
		}
	}

	tier.publishLedger(touched)
}

/*
publishLedger fans positions and this tier's wallet reading to the hub.
*/
func (tier *Tier) publishLedger(positions map[string]*Position) {
	ui.UI.WriteJSON(map[string]any{
		"positions": positions,
		"balances": map[string]any{
			tier.Type.String(): tier.wallet.Reading(),
		},
	})
}

func (tier *Tier) Hello() {
	positions := make(map[string]*Position)

	for _, position := range tier.positions.All() {
		if position == nil || position.ID == "" {
			continue
		}

		// Closed lots are frozen snapshots — do not re-mark them.
		if position.ClosedAt == nil {
			mark := position.Mark
			if mark == nil && tier.price != nil {
				mark = tier.price.TouchBid(position.Symbol)
			}
			if err := position.Update(mark); err != nil {
				errnie.Error(err)
			}
		}

		positions[position.ID] = position
	}

	tier.publishLedger(positions)
}

func (tier *Tier) Update(data *market.Update) {
	touched := make(map[string]*Position)

	for _, position := range tier.positions.Open(data.Symbol) {
		if err := position.Update(data.Price); err != nil {
			errnie.Error(err)
			continue
		}

		last, _ := tier.savedAt.Load(position.ID)
		savedAt, _ := last.(time.Time)

		if savedAt.IsZero() || time.Since(savedAt) > 30*time.Second {
			if err := tier.positions.Save(position); err != nil {
				errnie.Error(err)
			} else {
				tier.savedAt.Store(position.ID, time.Now())
			}
		}

		touched[position.ID] = position
	}

	if len(touched) > 0 {
		tier.publishLedger(touched)
	}
}

func (tier *Tier) Pending() iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		tier.pending.Range(func(_, value any) bool {
			return yield(value.(*Position))
		})
	}
}

func (tier *Tier) ClearPending(symbol string) {
	if symbol == "" {
		return
	}

	tier.pending.Delete(symbol)
}

func (tier *Tier) virgin() bool {
	return tier.WinRate == 0 && tier.Edge == 0
}

func (tier *Tier) ok() bool {
	return tier.WinRate > 0.5 && tier.Edge > 0
}

/*
Unlocked: virtual and paper always. Real needs paper win-rate above 50% and
edge above 0. Once real has history it must stay there too; after it slips,
paper has to lose that proof and earn it again before real entries resume.
*/
func (tier *Tier) Unlocked(paper *Tier) bool {
	if tier.Type != TierTypeReal {
		tier.unlocked = true
		return true
	}

	paperOK := paper != nil && paper.ok()

	if tier.halted {
		tier.noteHalt(paperOK)
		return tier.unlocked
	}

	if !paperOK {
		tier.unlocked = false
		tier.reprieve = false
		return false
	}

	if !tier.unlocked {
		if !tier.virgin() && !tier.ok() && !tier.reprieve {
			tier.halted = true
			tier.noteHalt(paperOK)
			return tier.unlocked
		}

		tier.unlocked = true
		tier.reprieve = false
		return true
	}

	if !tier.virgin() && !tier.ok() && !tier.reprieve {
		tier.halted = true
		tier.sawOK = paperOK
		tier.broke = false
		tier.reprieve = false
		tier.unlocked = false
		return false
	}

	tier.unlocked = true
	return true
}

/*
noteHalt keeps a failed real shut until paper goes cold and proves again.
*/
func (tier *Tier) noteHalt(paperOK bool) {
	if paperOK && tier.broke {
		tier.halted = false
		tier.broke = false
		tier.sawOK = false
		tier.reprieve = !tier.virgin() && !tier.ok()
		tier.unlocked = true
		return
	}

	if paperOK {
		tier.sawOK = true
	} else if tier.sawOK {
		tier.broke = true
	}

	tier.unlocked = false
}

func (tier *Tier) Enter(
	symbol string, path string, pathFound bool,
) error {
	if tier.Type == TierTypeReal && tier.live == nil {
		return errnie.Err(
			errnie.Forbidden,
			"[broker.Tier.Enter] real transport not armed",
			nil,
		)
	}

	if !tier.unlocked {
		return errnie.Err(
			errnie.Forbidden,
			"[broker.Tier.Enter] tier not unlocked",
			nil,
		)
	}

	if tier.positions.Has(symbol) {
		return errnie.Err(
			errnie.TooManyRequests,
			"[broker.Tier.Enter] position already exists",
			nil,
		)
	}

	// Size from this tier's venue wallet (virtual/paper/real all keep one).
	var wallet *market.Balance

	if tier.venue != nil {
		wallet = market.NewBalance(map[string]any{
			"quote":  system.Cfg.Market.QuoteCurrency,
			"assets": []*market.Asset{},
		})
		tier.venue.Balance().ToMarket(wallet)
	}

	qty, err := tier.price.Allocate(symbol, wallet)

	if err != nil || qty == nil || qty.Sign() <= 0 {
		if err != nil {
			return errnie.Error(err)
		}

		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.Enter] cannot size "+symbol,
			nil,
		)
	}

	pos := NewPosition(
		tier.ctx,
		symbol,
		tier.Type.String(),
		path,
		tier.price,
		tier.private,
		tier.settle,
	)

	if err := pos.Enter(path, qty, wallet); err != nil {
		return errnie.Error(err)
	}

	tier.positions.Add(symbol, pos, true)

	if err := tier.Send(pos, "enter"); err != nil {
		return errnie.Error(err)
	}

	// Fill Apply owns the first Save/publish. No-transport desks Apply a
	// book fill here so Enter never seeds outside Apply.
	if pos.transport == nil {
		if err := tier.fillsEnter(pos); err != nil {
			tier.positions.Drop(symbol)
			return err
		}
	}

	return nil
}

func (tier *Tier) fillsEnter(position *Position) error {
	if position == nil || position.EntryOrder == nil {
		return nil
	}

	qty, err := decimal.NewFromString(position.EntryOrder.Volume)
	if err != nil || qty.Sign() <= 0 {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.fillsEnter] bad entry volume",
			err,
		)
	}

	px := tier.price.FillAsk(position.Symbol, &qty)
	if px == nil || px.Sign() <= 0 {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.fillsEnter] no book price for "+position.Symbol,
			nil,
		)
	}

	now := time.Now().UTC()
	position.Apply(market.ExecutionData{
		ExecID:        "local-" + position.ID + "-enter",
		ExecType:      "trade",
		ClientOrderID: position.EntryOrder.ClOrdId,
		Symbol:        position.Symbol,
		Side:          "buy",
		LastQty:       &qty,
		LastPrice:     px,
		AvgPrice:      px,
		CumQty:        &qty,
		OrderStatus:   "filled",
		Timestamp:     now,
	})

	if err := tier.positions.Save(position); err != nil {
		return errnie.Error(err)
	}

	tier.publishLedger(map[string]*Position{position.ID: position})
	return nil
}

func (tier *Tier) Exit(
	position *Position,
) error {
	// Policy path arms Closing; exit fills in Apply stamp ClosedAt, then
	// settle Removes. Do not archive before the sell prints.
	if err := position.Exit(); err != nil {
		return errnie.Error(err)
	}

	if err := tier.Send(position, "exit"); err != nil {
		position.mu.Lock()
		position.Closing = false
		position.ExitTick = 0
		position.mu.Unlock()
		return errnie.Error(err)
	}

	if err := tier.positions.Save(position); err != nil {
		return errnie.Error(err)
	}

	tier.publishLedger(map[string]*Position{position.ID: position})
	return nil
}

func (tier *Tier) Send(position *Position, action string) error {
	order := position.EntryOrder

	if action == "exit" {
		order = position.ExitOrder
	}

	if tier.private == nil || order == nil {
		return nil
	}

	reqID := tier.reqSeq.Add(1)

	msg := map[string]any{
		"method": "add_order",
		"req_id": reqID,
		"params": map[string]any{
			"symbol":     position.Symbol,
			"side":       order.Type,
			"order_type": order.OrderType,
			"order_qty":  order.Volume,
			"cl_ord_id":  order.ClOrdId,
		},
	}

	buf, err := sonic.Marshal(msg)

	if err != nil {
		if action == "enter" {
			tier.positions.Drop(position.Symbol)
		}

		return errnie.Err(
			errnie.Internal,
			"[broker.Tier.Send] failed to marshal message",
			err,
		)
	}

	sent := &submission{
		position: position,
		action:   action,
		reqID:    reqID,
	}

	if tier.live != nil {
		buf, err = tier.live.Seal(buf)

		if err != nil {
			if action == "enter" {
				tier.positions.Drop(position.Symbol)
			}

			return errnie.Error(err)
		}
	}

	tier.inflight.Store(reqID, sent)

	if err := tier.private.Write(buf); err != nil {
		tier.inflight.Delete(reqID)

		if action == "enter" {
			tier.positions.Drop(position.Symbol)
		}

		if errnie.IsNotAcceptable(err) {
			return err
		}

		return errnie.Error(err)
	}

	return nil
}

func (tier *Tier) settle(position *Position) {
	if position == nil {
		return
	}

	// Exit fills landed — archive and hand to the model.
	if position.ClosedAt != nil {
		tier.positions.Remove(position.Symbol)
		tier.Recalculate()
		tier.pending.Store(position.Symbol, position)
		tier.publishLedger(map[string]*Position{position.ID: position})
		return
	}

	// Closing armed — send the exit and stay open until Apply flats the lot.
	if err := tier.Send(position, "exit"); err != nil {
		if errnie.IsNotFound(err) {
			now := time.Now().UTC()
			position.mu.Lock()
			position.ClosedAt = &now
			position.mu.Unlock()
			tier.positions.Remove(position.Symbol)
			tier.Recalculate()
			tier.pending.Store(position.Symbol, position)
			return
		}

		position.mu.Lock()
		position.Closing = false
		position.ExitTick = 0
		position.mu.Unlock()

		errnie.Error(err)
		return
	}

	if err := tier.positions.Save(position); err != nil {
		errnie.Error(err)
		return
	}

	tier.publishLedger(map[string]*Position{position.ID: position})
}

func (tier *Tier) Recalculate() {
	closed := tier.positions.Closed()

	if len(closed) == 0 {
		tier.WinRate = 0
		tier.Edge = 0
		return
	}

	wins := 0
	totalReturn := 0.0
	count := 0

	for _, pos := range closed {
		if pos == nil || pos.ClosedAt == nil || pos.PnLPct == nil {
			continue
		}

		pnlPct, _ := pos.PnLPct.Float64()
		totalReturn += pnlPct

		if pnlPct > 0 {
			wins++
		}

		count++
	}

	if count == 0 {
		tier.WinRate = 0
		tier.Edge = 0
		return
	}

	tier.WinRate = float64(wins) / float64(count)
	tier.Edge = totalReturn / float64(count)
	tier.reprieve = false

	if tier.Type != TierTypeReal {
		return
	}

	if tier.ok() {
		tier.halted = false
		return
	}

	tier.halted = true
	tier.sawOK = false
	tier.broke = false
	tier.unlocked = false
}

/*
Run owns the private ingress for this tier. Public frames are read once on Desk.
*/
func (tier *Tier) Run() {
	if tier.private == nil || tier.venue == nil {
		return
	}

	if tier.live != nil {
		if err := tier.live.Arm(); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[broker.Tier] private subscribe failed", err,
			))
		}
	}

	// Resend exits for lots restored mid-close once private ingress is up.
	for _, position := range tier.positions.Open() {
		if position != nil && position.Closing && position.ClosedAt == nil {
			tier.settle(position)
		}
	}

	for {
		select {
		case <-tier.ctx.Done():
			return
		default:
		}

		buf, err := tier.private.Read()

		if err != nil {
			if tier.ctx.Err() != nil {
				return
			}

			continue
		}

		tier.venue.Read(buf)
		tier.notePrivate(buf)
	}
}

/*
fillReport keeps trade prints. Status-only execution events are not fills.
*/
func (tier *Tier) fillReport(execution *market.ExecutionData) bool {
	if execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		return false
	}

	return execution.ExecType == "" || execution.ExecType == "trade"
}

func (tier *Tier) ack(order *kraken.Order) {
	if order == nil {
		return
	}

	if order.Success {
		if order.ReqID != 0 {
			tier.inflight.Delete(order.ReqID)
		}

		return
	}

	tier.reject(order.ReqID, order.Error)
}

func (tier *Tier) reject(reqID int64, reason string) {
	if reqID == 0 {
		errnie.Error(errnie.Err(
			errnie.NotAcceptable, "[broker.Tier] order rejected without req_id", nil,
		).With("reason", reason))

		return
	}

	value, ok := tier.inflight.LoadAndDelete(reqID)

	if !ok {
		return
	}

	tier.dropRejected(value.(*submission), reason)
}

func (tier *Tier) dropRejected(sent *submission, reason string) {
	if sent == nil || sent.position == nil {
		return
	}

	position := sent.position

	if sent.action == "exit" {
		position.mu.Lock()
		position.Closing = false
		position.ExitTick = 0
		position.mu.Unlock()

		errnie.Error(errnie.Err(
			errnie.NotAcceptable, "[broker.Tier] exit rejected", nil,
		).With("symbol", position.Symbol, "reason", reason))

		return
	}

	tier.positions.Drop(position.Symbol)

	errnie.Error(errnie.Err(
		errnie.NotAcceptable, "[broker.Tier] entry rejected", nil,
	).With("symbol", position.Symbol, "reason", reason))

	tier.Hello()
}

/*
notePrivate logs a rejected private subscription. Order rejects take ack.
*/
func (tier *Tier) notePrivate(buf []byte) {
	var head struct {
		Method  string `json:"method"`
		Success *bool  `json:"success"`
		Error   string `json:"error"`
	}

	if err := sonic.Unmarshal(buf, &head); err != nil {
		return
	}

	if head.Method != "subscribe" || head.Success == nil || *head.Success {
		return
	}

	errnie.Error(errnie.Err(
		errnie.IO, "[broker.Tier] private subscribe rejected", nil,
	).With("error", head.Error))
}
