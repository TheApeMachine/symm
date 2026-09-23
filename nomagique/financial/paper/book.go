package paper

import (
	"bytes"
	"context"
	"encoding/json"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* BookServer owns each symbol's replayed book and latest instrument record. */
type BookServer struct {
	*runtime.System
	books     map[string]*book.Book
	pairs     map[string]json.RawMessage
	reconcile *spot.BookManager
	out       []byte
}

func NewBook(ctx context.Context) *BookServer {
	return &BookServer{
		System:    runtime.NewSystem(ctx, "paper.book"),
		books:     make(map[string]*book.Book),
		pairs:     make(map[string]json.RawMessage),
		reconcile: spot.NewBookManager(),
	}
}

func (server *BookServer) Write(ctx context.Context, call Book_write) error {
	frames, err := call.Args().Frame()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Validation, "paper.book: frame", err,
		))
	}

	server.out = []byte(`{"symbol":""}`)
	frame, err := single(frames)

	if err != nil {
		return server.Error(err)
	}

	if frame == nil {
		return nil
	}

	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.UseNumber()

	var envelope struct {
		Channel string          `json:"channel"`
		Type    string          `json:"type"`
		Data    json.RawMessage `json:"data"`
	}

	if err := decoder.Decode(&envelope); err != nil {
		return server.Error(errnie.Err(
			errnie.Validation, "paper.book: decode frame", err,
		))
	}

	if envelope.Channel == "instrument" {
		return server.instruments(envelope.Data)
	}

	if envelope.Channel != "level3" {
		return nil
	}

	return server.level3(envelope.Type, envelope.Data, call.Args().Depth())
}

/* single is the one frame that arrived this evaluation, or nil when none did. */
func single(frames capnp.DataList) ([]byte, error) {
	var frame []byte

	for index := range frames.Len() {
		arrived, err := frames.At(index)

		if err != nil {
			return nil, errnie.Err(errnie.Validation, "paper.book: frame", err)
		}

		if len(arrived) == 0 {
			continue
		}

		if frame != nil {
			return nil, errnie.Err(errnie.Validation, "paper.book: one evaluation takes one frame", nil)
		}
		frame = arrived
	}
	return frame, nil
}

func (server *BookServer) instruments(data json.RawMessage) error {
	var catalog struct {
		Pairs []json.RawMessage `json:"pairs"`
	}

	if err := json.Unmarshal(data, &catalog); err != nil {
		return server.Error(errnie.Err(
			errnie.Validation, "paper.book: decode instrument data", err,
		))
	}

	for _, pair := range catalog.Pairs {
		var named struct {
			Symbol string `json:"symbol"`
		}

		if err := json.Unmarshal(pair, &named); err != nil || named.Symbol == "" {
			return server.Error(errnie.Err(
				errnie.Validation,
				"paper.book: instrument pair without symbol",
				err,
			))
		}

		server.pairs[named.Symbol] = pair
	}

	return nil
}

func (server *BookServer) level3(kind string, data json.RawMessage, depth int64) error {
	if depth <= 0 {
		return server.Error(errnie.Err(
			errnie.Validation,
			"paper.book: level3 frames need the subscribed depth",
			nil,
		))
	}

	// A replayed record carries its one entry as an object, a live frame as an array.
	trimmed := bytes.TrimSpace(data)

	if bytes.HasPrefix(trimmed, []byte("{")) {
		trimmed = append(append([]byte("["), trimmed...), ']')
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var entries []map[string]any

	if err := decoder.Decode(&entries); err != nil {
		return server.Error(errnie.Err(
			errnie.Validation, "paper.book: decode level3 data", err,
		))
	}

	// One evaluation reports one market. The exchange sends one symbol per
	// level3 frame; a frame carrying more would lose all but one.
	if len(entries) != 1 {
		return server.Error(errnie.Err(
			errnie.Validation,
			"paper.book: a level3 frame must carry exactly one symbol",
			nil,
		))
	}

	entry := entries[0]
	symbol, found := entry["symbol"].(string)

	if !found || symbol == "" {
		return server.Error(errnie.Err(
			errnie.Validation, "paper.book: level3 entry without symbol", nil,
		))
	}

	if kind == "snapshot" {
		fresh := book.New()
		fresh.Name = symbol
		fresh.MaxDepth = int(depth)
		fresh.EnableMaxDepth = true
		server.books[symbol] = fresh
	}

	held := server.books[symbol]

	if held == nil {
		return nil
	}

	// A book that no longer reconciles with the exchange's checksum is not
	// the book the exchange held; it stays unknown until the next snapshot.
	if err := server.reconcile.UpdateL3(held, entry); err != nil {
		errnie.Warn(
			"paper.book: book lost until next snapshot",
			"symbol", symbol,
			"cause", err.Error(),
		)

		delete(server.books, symbol)
		return nil
	}
	return server.report(symbol, held)
}

func (server *BookServer) report(symbol string, held *book.Book) error {
	market := struct {
		Symbol string          `json:"symbol"`
		Bids   [][2]string     `json:"bids"`
		Asks   [][2]string     `json:"asks"`
		Pair   json.RawMessage `json:"pair,omitempty"`
	}{
		Symbol: symbol,
		Bids:   [][2]string{},
		Asks:   [][2]string{},
		Pair:   server.pairs[symbol],
	}

	for level := held.BestBid(); level != nil; level = level.Lower {
		market.Bids = append(
			market.Bids,
			[2]string{level.Price.String(), level.Quantity.String()},
		)
	}

	for level := held.BestAsk(); level != nil; level = level.Higher {
		market.Asks = append(
			market.Asks,
			[2]string{level.Price.String(), level.Quantity.String()},
		)
	}

	out, err := json.Marshal(market)

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Internal, "paper.book: encode market", err,
		))
	}

	server.out = out
	return nil
}

func (server *BookServer) Done(ctx context.Context, call Book_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Internal, "paper.book: allocate result", err,
		))
	}

	if err := results.SetOut(server.out); err != nil {
		return server.Error(errnie.Err(errnie.Internal, "paper.book: emit", err))
	}

	server.out = nil
	return nil
}
