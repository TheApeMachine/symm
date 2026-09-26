package paper

import (
	"bytes"
	"context"
	"encoding/json"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* BookServer owns each symbol's canonical reconciled order book. */
type BookServer struct {
	*runtime.System
	markets    map[string]*marketBook
	reconcile  *spot.BookManager
	values     [29]float64
	present    [29]bool
	reconciled uint64
	symbol     string
	updated    bool
}

func NewBook(ctx context.Context) *BookServer {
	return &BookServer{
		System:    runtime.NewSystem(ctx, "paper.book"),
		markets:   make(map[string]*marketBook),
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

	server.values, server.present = [29]float64{}, [29]bool{}
	server.symbol, server.updated = "", false
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

	if envelope.Channel == "trade" {
		return server.match(envelope.Data)
	}

	if envelope.Channel != "level3" {
		return server.identify(envelope.Data)
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
	if err := server.flow(entry, kind); err != nil {
		return err
	}
	symbol, found := entry["symbol"].(string)

	if !found || symbol == "" {
		return server.Error(errnie.Err(
			errnie.Validation, "paper.book: level3 entry without symbol", nil,
		))
	}
	server.symbol = symbol

	market := server.marketBook(symbol)
	if kind == "snapshot" {
		market.matched = [2]*decimal.Decimal{}
		fresh := book.New()
		fresh.Name = symbol
		fresh.MaxDepth = int(depth)
		fresh.EnableMaxDepth = true
		market.book = fresh
	}

	held := market.book

	if held == nil {
		return nil
	}

	prior := market.touch()

	// A book that no longer reconciles with the exchange's checksum is not
	// the book the exchange held; it stays unknown until the next snapshot.
	if err := server.reconcileL3(held, entry); err != nil {
		errnie.Warn(
			"paper.book: book lost until next snapshot",
			"symbol", symbol,
			"cause", err.Error(),
		)

		market.book = nil
		market.matched = [2]*decimal.Decimal{}
		return nil
	}
	server.project(held)
	if kind != "snapshot" {
		server.touch(market, prior)
	}
	market.matched = [2]*decimal.Decimal{}
	server.reconciled++
	server.updated = true

	return nil
}

func (server *BookServer) Done(ctx context.Context, call Book_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return server.Error(errnie.Err(
			errnie.Internal, "paper.book: allocate result", err,
		))
	}

	results.SetReconciled(server.reconciled)

	if held := server.markets[server.symbol]; held != nil && held.book != nil {
		market, err := results.NewMarket()

		if err != nil {
			return errnie.Error(err)
		}

		if err := server.market(market, held.book); err != nil {
			return err
		}
	}
	values, err := results.NewValues(int32(len(server.values)))

	if err != nil {
		return errnie.Error(err)
	}
	present, err := results.NewPresent(int32(len(server.present)))

	if err != nil {
		return errnie.Error(err)
	}
	for index, value := range server.values {
		values.Set(index, value)
		present.Set(index, server.present[index])
	}

	return nil
}
