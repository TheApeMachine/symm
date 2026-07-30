package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

const (
	PublicWebSocketURL  = "wss://ws.kraken.com/v2"
	PrivateWebSocketURL = "wss://ws-auth.kraken.com/v2"
	Level3WebSocketURL  = "wss://ws-l3.kraken.com/v2"
)

var entityMap = map[string]func([]byte) any{
	"ticker":     func(buf []byte) any { return kraken.NewTicker(buf) },
	"book":       func(buf []byte) any { return kraken.NewBook(buf) },
	"trade":      func(buf []byte) any { return kraken.NewTrade(buf) },
	"level3":     func(buf []byte) any { return kraken.NewLevel3(buf) },
	"instrument": func(buf []byte) any { return kraken.NewInstrument(buf) },
}

/*
Live is one spot websocket session: SDK client, channel fan-out, auth/nonce,
and Sub* resubscribe after the SDK reconnects.
*/
type Live struct {
	*types.Actor
	status    types.Status
	ctx       context.Context
	cancel    context.CancelFunc
	client    *spot.WebSocket
	simulator *Simulator
	books     *spot.BookManager
	bookMu    sync.RWMutex
	level3    *level3Ledger
	isLevel3  bool
	symbols   []string
	auth      bool
	nonce     *AuthNonce
	nonceErr  error
	ready     func() error
	readyGate *types.ReadyFuture
	roots     map[string]*types.Subscription
	priceIncr sync.Map
	waiting   sync.Map
	resyncing sync.Map
}

/*
New opens a spot websocket session and wires SDK callbacks in the constructor.
*/
func New(
	ctx context.Context,
	simulator *Simulator,
	auth bool,
	endpoint string,
) *Live {
	ctx, cancel := context.WithCancel(ctx)

	live := &Live{
		ctx:       ctx,
		cancel:    cancel,
		status:    types.INITIALIZING,
		simulator: simulator,
		client:    spot.NewWebSocket(),
		books:     spot.NewBookManager(),
		auth:      auth,
		readyGate: types.NewReadyFuture(),
		roots: map[string]*types.Subscription{
			"ticker":     types.NewSubscription(),
			"book":       types.NewSubscription(),
			"trade":      types.NewSubscription(),
			"level3":     types.NewSubscription(),
			"instrument": types.NewSubscription(),
			"balances":   types.NewSubscription(),
			"executions": types.NewSubscription(),
			"add_order":  types.NewSubscription(),
		},
	}

	live.Actor = types.NewActor(ctx, "live", nil)

	for name, root := range live.roots {
		live.AddRoot(name, root)
	}

	live.Actor.Initialize()
	live.client.URL = endpoint
	live.books.OnCreateBook.Recurring(func(event *callback.Event[*book.Book]) {
		managed := event.Data

		if managed == nil {
			return
		}

		if live.isLevel3 {
			managed.EnableMaxDepth = false
			managed.NoBookCrossing = false
			return
		}

		managed.NoBookCrossing = false
	})

	if auth {
		nonce, err := processAuthNonce()
		live.nonce = nonce
		live.nonceErr = err
		live.wireCredentials()
	}

	if endpoint == Level3WebSocketURL {
		live.isLevel3 = true
		live.level3 = newLevel3Ledger()
	}

	live.client.OnSent.Recurring(func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
		if !live.ownsBookEvent(event.Data.Bytes()) {
			return
		}

		if err := live.ingestBookManager(event); err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"websocket: sent-frame book ingest failed: "+err.Error(),
				err,
			))
		}
	})

	live.client.OnReceived.Recurring(func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
		raw := event.Data.Bytes()

		channel := utils.GetString(raw, "channel")

		if channel == "" {
			if method := utils.GetString(raw, "method"); method == "add_order" {
				channel = method
			}
		}

		if channel == "" {
			if message := utils.GetString(raw, "error"); message != "" {
				errnie.Error(errnie.Err(errnie.Validation, message, nil))
			}

			return
		}

		if channel == "book" || channel == "level3" {
			if !live.ownsBookChannel(channel) {
				return
			}

			var err error

			if channel == "level3" {
				err = live.ingestLevel3(raw)
			} else {
				err = live.ingestBookManager(event)
			}

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"websocket: book update failed: "+err.Error(),
					err,
				))
				live.scheduleBookRecovery(channel, raw)
			}
		}

		if channel == "status" || channel == "heartbeat" || (live.isLevel3 && channel == "level3") {
			return
		}

		if channel == "balances" || channel == "executions" || channel == "add_order" {
			live.roots[channel].Send(raw)
			return
		}

		if entity, ok := entityMap[channel]; ok {
			entity := entity(raw)

			switch channel {
			case "instrument":
				live.rememberIncrements(entity.(*kraken.Instrument))
			case "book":
				if errnie.Error(live.stampBook(entity.(*kraken.Book))) != nil {
					return
				}
			}

			live.roots[channel].Send(entity)
		}
	})

	live.client.OnConnected.Recurring(func(event *callback.Event[any]) {
		live.onConnected()
	})

	if auth {
		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			live.onAuthenticated()
		})
	}

	return live
}

func (live *Live) isBookEvent(raw []byte) bool {
	text := string(raw)

	return strings.Contains(text, `"channel":"book"`) ||
		strings.Contains(text, `"channel":"level3"`)
}

func (live *Live) ownsBookEvent(raw []byte) bool {
	if !live.isBookEvent(raw) {
		return false
	}

	channel := utils.GetString(raw, "channel")

	if channel == "" {
		channel = utils.GetString(raw, "params", "channel")
	}

	return live.ownsBookChannel(channel)
}

func (live *Live) ingestLevel3(raw []byte) error {
	if live == nil || live.books == nil || live.level3 == nil || len(raw) == 0 {
		return nil
	}

	live.bookMu.Lock()
	defer live.bookMu.Unlock()

	return live.level3.Apply(live.books, raw)
}

func (live *Live) ownsBookChannel(channel string) bool {
	if live == nil {
		return false
	}

	if live.isLevel3 {
		return channel == "level3"
	}

	return channel == "book"
}

func (live *Live) ingestBookManager(
	event *callback.Event[*sdkkraken.WebSocketMessage],
) (err error) {
	if live == nil || live.books == nil || event == nil || event.Data == nil {
		return nil
	}

	raw := event.Data.Bytes()
	channel := utils.GetString(raw, "channel")

	if channel == "" {
		channel = utils.GetString(raw, "params", "channel")
	}

	symbols := live.bookSymbols(channel, raw)
	method := utils.GetString(raw, "method")
	frameType := utils.GetString(raw, "type")

	if method == "subscribe" {
		live.setWaiting(channel, utils.GetStringSlice(raw, "params", "symbol"))
	}

	if live.shouldWaitForSnapshot(channel, method, frameType, symbols) {
		return nil
	}

	live.bookMu.Lock()
	defer live.bookMu.Unlock()

	if live.missingBookStateLocked(channel, raw) {
		depth := live.channelDepth(channel)
		live.setWaiting(channel, symbols)
		live.resetBooksLocked(symbols, depth)

		return errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"websocket: waiting for replacement snapshot channel=%s symbols=%v",
				channel,
				symbols,
			),
			nil,
		)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			depth := live.channelDepth(channel)
			live.setWaiting(channel, symbols)
			live.resetBooksLocked(symbols, depth)
			err = fmt.Errorf(
				"book manager panic channel=%s symbols=%v recovered=%v",
				channel,
				symbols,
				recovered,
			)
			errnie.Error(errnie.Err(
				errnie.Validation,
				"websocket: recovered from SDK book panic",
				err,
			))
		}
	}()

	err = live.books.Update(event)

	if err != nil {
		depth := live.channelDepth(channel)
		live.setWaiting(channel, symbols)
		live.resetBooksLocked(symbols, depth)
		return err
	}

	if frameType == "snapshot" {
		live.clearWaiting(channel, symbols)
	}

	return err
}

func (live *Live) missingBookStateLocked(channel string, raw []byte) bool {
	if live == nil || live.books == nil || len(raw) == 0 {
		return false
	}

	if channel == "book" {
		frame := kraken.NewBook(raw)

		for _, row := range frame.Data {
			if row.Type == "snapshot" {
				continue
			}

			managed := live.books.GetBook(row.Symbol)

			if managed == nil {
				return true
			}

			if live.missingBookLevels(managed.Bids.Levels, row.Bids) {
				return true
			}

			if live.missingBookLevels(managed.Asks.Levels, row.Asks) {
				return true
			}
		}

		return false
	}

	if channel == "level3" {
		frame := kraken.NewLevel3(raw)

		for _, row := range frame.Data {
			if row.Type == "snapshot" {
				continue
			}

			managed := live.books.GetBook(row.Symbol)

			if managed == nil {
				return true
			}

			if live.missingLevel3Orders(managed.Bids.Levels, row.Bids) {
				return true
			}

			if live.missingLevel3Orders(managed.Asks.Levels, row.Asks) {
				return true
			}
		}
	}

	return false
}

func (live *Live) missingBookLevels(
	levels map[string]*book.Level,
	rows []kraken.BookLevel,
) bool {
	for _, row := range rows {
		if row.Qty > 0 {
			continue
		}

		if levels[row.Price.String()] == nil {
			return true
		}
	}

	return false
}

func (live *Live) missingLevel3Orders(
	levels map[string]*book.Level,
	rows []kraken.Level3Order,
) bool {
	for _, row := range rows {
		event := row.Event

		if event == "" {
			event = "add"
		}

		if event == "add" || row.LimitPrice == nil {
			continue
		}

		level := levels[row.LimitPrice.String()]

		if level == nil {
			return true
		}

		if event == "delete" || event == "modify" {
			if !live.levelHasOrder(level, row.OrderID) {
				return true
			}
		}
	}

	return false
}

func (live *Live) levelHasOrder(level *book.Level, orderID string) bool {
	if level == nil || orderID == "" {
		return false
	}

	for _, order := range level.Queue() {
		if order != nil && order.ID == orderID {
			return true
		}
	}

	return false
}

func (live *Live) rememberIncrements(frame *kraken.Instrument) {
	if frame == nil {
		return
	}

	for index := range frame.Data.Pairs {
		pair := frame.Data.Pairs[index]
		live.priceIncr.Store(pair.Symbol, pair.PriceIncrement)
	}
}

func (live *Live) stampBook(frame *kraken.Book) error {
	if frame == nil {
		return errnie.Err(errnie.Validation, "websocket: book required to stamp", nil)
	}

	for index := range frame.Data {
		value, ok := live.priceIncr.Load(frame.Data[index].Symbol)

		if !ok {
			return errnie.Err(
				errnie.Validation,
				"websocket: price increment required for "+frame.Data[index].Symbol,
				nil,
			)
		}

		increment := value.(decimal.Decimal)
		frame.Data[index].PriceIncrement = &increment
	}

	return nil
}

func (live *Live) scheduleBookRecovery(channel string, raw []byte) {
	if live == nil || len(raw) == 0 {
		return
	}

	symbols := live.recoverySymbols(channel, raw)
	live.setWaiting(channel, symbols)

	if _, loaded := live.resyncing.LoadOrStore(channel, struct{}{}); loaded {
		return
	}

	go func(symbols []string) {
		defer live.resyncing.Delete(channel)

		if err := live.recoverTransport(channel, symbols); err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"websocket: transport recovery failed",
				err,
			))
		}
	}(append([]string(nil), symbols...))
}

func (live *Live) recoverySymbols(channel string, raw []byte) []string {
	if len(live.symbols) > 0 {
		return append([]string(nil), live.symbols...)
	}

	return live.bookSymbols(channel, raw)
}

func (live *Live) waitingKey(channel, symbol string) string {
	return channel + ":" + symbol
}

func (live *Live) setWaiting(channel string, symbols []string) {
	if live == nil || channel == "" {
		return
	}

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		live.waiting.Store(live.waitingKey(channel, symbol), struct{}{})
	}
}

func (live *Live) clearWaiting(channel string, symbols []string) {
	if live == nil || channel == "" {
		return
	}

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		live.waiting.Delete(live.waitingKey(channel, symbol))
	}
}

func (live *Live) isWaiting(channel, symbol string) bool {
	if live == nil || channel == "" || symbol == "" {
		return false
	}

	_, ok := live.waiting.Load(live.waitingKey(channel, symbol))
	return ok
}

func (live *Live) shouldWaitForSnapshot(
	channel string,
	method string,
	frameType string,
	symbols []string,
) bool {
	if live == nil || channel == "" || method == "subscribe" || frameType == "snapshot" {
		return false
	}

	for _, symbol := range symbols {
		if live.isWaiting(channel, symbol) {
			return true
		}
	}

	return false
}

func (live *Live) bookSymbols(channel string, raw []byte) []string {
	if channel == "book" {
		frame := kraken.NewBook(raw)
		symbols := make([]string, 0, len(frame.Data))

		for _, row := range frame.Data {
			if row.Symbol == "" {
				continue
			}

			symbols = append(symbols, row.Symbol)
		}

		return slices.Compact(symbols)
	}

	if channel == "level3" {
		frame := kraken.NewLevel3(raw)
		symbols := make([]string, 0, len(frame.Data))

		for _, row := range frame.Data {
			if row.Symbol == "" {
				continue
			}

			symbols = append(symbols, row.Symbol)
		}

		return slices.Compact(symbols)
	}

	return nil
}

func (live *Live) recoverTransport(channel string, symbols []string) error {
	if live == nil {
		return nil
	}

	depth := live.channelDepth(channel)
	live.resetBooks(symbols, depth)

	if live.client == nil {
		return nil
	}

	live.client.DoReconnect = false
	_ = live.client.Disconnect()
	live.readyGate = types.NewReadyFuture()
	live.client.Reconnect()

	return nil
}

func (live *Live) resetBooks(symbols []string, depth int) {
	if live == nil || live.books == nil || depth <= 0 {
		return
	}

	live.bookMu.Lock()
	defer live.bookMu.Unlock()
	live.resetBooksLocked(symbols, depth)
}

func (live *Live) resetBooksLocked(symbols []string, depth int) {
	if live == nil || live.books == nil || depth <= 0 {
		return
	}

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		managed := live.books.CreateBook(symbol, depth)

		if live.isLevel3 {
			managed.EnableMaxDepth = false
			managed.NoBookCrossing = false

			if live.level3 != nil {
				delete(live.level3.orders, symbol)
				live.level3.waiting[symbol] = struct{}{}
			}
		}
	}
}

func (live *Live) channelDepth(channel string) int {
	depth := viper.GetInt("market.book.depth")

	if channel == "level3" {
		depth = viper.GetInt("market.l3_depth")
	}

	if depth <= 0 {
		return 10
	}

	return depth
}

func (live *Live) Status() types.Status {
	return live.status
}

func (live *Live) wireCredentials() {
	if live.client == nil || !live.auth {
		return
	}

	live.client.REST.PublicKey = os.Getenv("KRAKEN_API_KEY")
	live.client.REST.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

	if live.nonceErr != nil || live.nonce == nil {
		return
	}

	// Private and every Level3 batch authenticate with the same key; they
	// must share one monotonic nonce sequence or concurrent token fetches
	// collide (EAPI:Invalid nonce).
	live.client.REST.Nonce = live.nonce.Next
}

func (live *Live) onConnected() {
	if !live.auth {
		if live.ready != nil {
			if err := live.ready(); err != nil {
				live.status = types.ERROR
				live.readyGate.Resolve(err)
				errnie.Error(errnie.Err(
					errnie.Validation,
					"websocket: public resubscribe failed",
					err,
				))

				return
			}
		}

		live.status = types.READY
		live.readyGate.Resolve(nil)

		return
	}

	if errnie.Error(live.authenticate()) != nil {
		live.status = types.ERROR
		live.readyGate.Resolve(types.ClosedError{Component: "websocket:auth"})
	}
}

func (live *Live) onAuthenticated() {
	if live.isLevel3 && len(live.symbols) > 0 && live.SubscribeLevel3(
		live.symbols,
		viper.GetInt("market.l3_depth"),
	) != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: level3 book subscription failed",
			nil,
		))
		live.status = types.ERROR
		live.readyGate.Resolve(types.ClosedError{Component: "websocket:level3"})

		return
	}

	if live.ready != nil {
		if err := live.ready(); err != nil {
			live.status = types.ERROR
			live.readyGate.Resolve(err)
			errnie.Error(errnie.Err(
				errnie.Validation,
				"websocket: private resubscribe failed",
				err,
			))

			return
		}
	}

	live.status = types.READY
	live.readyGate.Resolve(nil)
}

func (live *Live) authenticate() (err error) {
	if live.nonceErr != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: auth nonce unavailable",
			live.nonceErr,
		))
	}

	if err = live.client.Authenticate(); err != nil && !strings.Contains(err.Error(), "Invalid nonce") {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: authentication failed",
			err,
		))
	}

	if err == nil {
		return nil
	}

	if live.nonce != nil {
		live.nonce.Bump()
	}

	return live.client.Authenticate()
}

/*
SubscribeLevel3 ensures each symbol has an SDK book before the first snapshot or
update arrives, then delegates subscription to Kraken's standard SubL3 helper.
*/
func (live *Live) SubscribeLevel3(symbols []string, depth int) error {
	if live == nil || live.client == nil {
		return errnie.Err(errnie.NotFound, "websocket: level3 transport unavailable", nil)
	}

	if depth <= 0 {
		depth = 10
	}

	live.ensureBooks(symbols, depth)

	return live.client.SubL3(symbols, depth)
}

/*
ensureBooks creates missing SDK books under the write lease so book and level3
snapshots never arrive before their target entries exist in the manager.
*/
func (live *Live) ensureBooks(symbols []string, depth int) {
	if live == nil || live.books == nil || depth <= 0 {
		return
	}

	live.bookMu.Lock()
	defer live.bookMu.Unlock()

	for _, symbol := range symbols {
		if symbol == "" || live.books.GetBook(symbol) != nil {
			continue
		}

		managed := live.books.CreateBook(symbol, depth)

		if live.isLevel3 {
			managed.EnableMaxDepth = false
			managed.NoBookCrossing = false
		}
	}
}

/*
PeekBook calls fn while holding this transport's book read lease.
*/
func (live *Live) PeekBook(symbol string, fn func(*book.Book)) bool {
	if live == nil || live.books == nil || fn == nil || symbol == "" {
		return false
	}

	live.bookMu.RLock()
	defer live.bookMu.RUnlock()

	symbolBook := live.books.GetBook(symbol)

	if symbolBook == nil {
		return false
	}

	fn(symbolBook)

	return true
}

/*
ApplyLevel3 feeds one raw Level3 websocket payload through the standard SDK book
manager path so tests can update books synchronously without a live socket.
*/
func (live *Live) ApplyLevel3(payload []byte) error {
	if live == nil {
		return nil
	}

	if len(payload) == 0 {
		return errnie.Err(
			errnie.Validation,
			"websocket: level3 payload is empty",
			nil,
		)
	}

	event := &callback.Event[*sdkkraken.WebSocketMessage]{
		Data: sdkkraken.NewWebSocketMessage(payload),
	}

	if utils.GetString(payload, "method") == "subscribe" {
		return live.ingestBookManager(event)
	}

	return live.ingestLevel3(payload)
}

func (live *Live) Initialize() error {
	errnie.Info("initializing live")

	if err := live.client.Connect(); err != nil {
		live.status = types.ERROR
		live.readyGate.Resolve(err)

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: connect failed",
			err,
		))
	}

	return nil
}

/*
Ready returns the future that resolves once auth and required subs complete.
*/
func (live *Live) Ready() *types.ReadyFuture {
	if live.readyGate == nil {
		live.readyGate = types.NewReadyFuture()
	}

	return live.readyGate
}

/*
Root returns the Actor fan-out for this session.
*/
func (live *Live) Root() *types.Actor {
	return live.Actor
}

func (live *Live) Client() *spot.WebSocket {
	return live.client
}

func (live *Live) Books() *spot.BookManager {
	return live.books
}

func (live *Live) Write(params json.Marshaler) error {
	raw, err := params.MarshalJSON()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: write marshal failed",
			err,
		))
	}

	started := time.Now()

	err = live.client.WriteMessage(
		gorillawebsocket.TextMessage, raw,
	)

	if live.simulator != nil {
		live.simulator.Record(WEBSOCKET, time.Since(started))
	}

	return errnie.Error(err)
}

func (live *Live) do(options spot.RequestOptions) ([]byte, error) {
	started := time.Now()

	request, err := live.client.REST.NewRequest(options)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	resp, err := request.Do()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"Kraken REST request failed",
			err,
		))
	}

	errors := utils.GetStringSlice(resp.Body, "error")

	if len(errors) > 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			errors[0],
			nil,
		))
	}

	if resp.StatusCode != 200 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"websocket.Live.do[%d]: %s",
				resp.StatusCode,
				resp.Body,
			),
			nil,
		))
	}

	if live.simulator != nil {
		live.simulator.Record(REST, time.Since(started))
	}

	return resp.Body, nil
}

func (live *Live) Post(
	path string, params json.Marshaler,
) ([]byte, error) {
	return live.do(spot.RequestOptions{
		Auth:   live.auth,
		Path:   path,
		Method: "POST",
		Body:   params,
	})
}

func (live *Live) Close() {
	live.cancel()

	if live.client.IsActive() {
		errnie.Error(live.client.Disconnect())
	}
}
