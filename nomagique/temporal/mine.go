package temporal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	random "math/rand/v2"

	"github.com/theapemachine/errnie"
)

/* TapeCursor addresses one record within an ingress frame; sequence is numeric. */
type TapeCursor struct {
	Sequence int64 `json:"sequence"`
	Record   int   `json:"record"`
}

/*
	MinedEvent preserves precursor, ignition, extremum and later confirmation.

A is absent when the first leg has no observed precursor; it is never set to B.
*/
type MinedEvent struct {
	Symbol    string      `json:"symbol"`
	Precursor TapeCursor  `json:"precursor"`
	A         *TapeCursor `json:"a,omitempty"`
	B         TapeCursor  `json:"b"`
	C         TapeCursor  `json:"c"`
	D         TapeCursor  `json:"d"`
	Seed      string      `json:"seed,omitempty"`
	Excursion float64     `json:"excursion"`
}

type minedPath struct {
	excursion *ExcursionServer
	cursors   []TapeCursor
}

type miningStream struct{ session, endpoint, channel string }
type miningSymbol struct {
	stream miningStream
	symbol string
}

/* MineServer expands channel records and owns independent paths per instrument. */
type MineServer struct {
	paths     map[miningSymbol]*minedPath
	sequences map[miningStream]int64
	out       []byte
	batch     []byte
}

func NewMine() *MineServer {
	return &MineServer{paths: make(map[miningSymbol]*minedPath), sequences: make(map[miningStream]int64)}
}

/* Write mines a complete archived frame, retaining every symbol and record. */
func (server *MineServer) Write(ctx context.Context, call Mine_write) error {
	server.out = nil
	server.batch = nil
	args := call.Args()
	payload, err := args.Payload()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: payload", err))
	}
	session, err := args.Session()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: session", err))
	}
	endpoint, err := args.Endpoint()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: endpoint", err))
	}
	channel, err := args.Channel()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: channel", err))
	}
	priceField, err := args.PriceField()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: price field", err))
	}

	if session == "" || endpoint == "" || channel == "" || priceField == "" || args.Sequence() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: session, endpoint, channel, price field and sequence are required", nil))
	}

	var envelope struct {
		Channel string          `json:"channel"`
		Data    json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(payload, &envelope); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: decode frame", err))
	}

	// Other channels shape their data as they like; only the mined one is read.
	if envelope.Channel != channel {
		return nil
	}
	var frame struct{ Data []map[string]json.RawMessage }

	if err := json.Unmarshal(envelope.Data, &frame.Data); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: decode "+channel+" data", err))
	}

	stream := miningStream{session, endpoint, channel}

	if previous, found := server.sequences[stream]; found && args.Sequence() <= previous {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: capture sequence did not advance", nil))
	}

	// Validate the entire frame before changing any instrument state.
	readings, err := readPrices(frame.Data, priceField)

	if err != nil {
		return err
	}
	events := make([]MinedEvent, 0)

	for _, reading := range readings {
		key := miningSymbol{stream, reading.symbol}
		path := server.paths[key]

		if path == nil {
			path = &minedPath{excursion: NewExcursion(ctx)}
			server.paths[key] = path
		}
		cursor := TapeCursor{args.Sequence(), reading.record}
		path.cursors = append(path.cursors, cursor)

		if err := path.excursion.Step(reading.value); err != nil {
			return err
		}

		if !path.excursion.reported.found {
			continue
		}
		event, err := path.event(reading.symbol, cursor)

		if err != nil {
			return err
		}
		events = append(events, event)
		path.excursion.reported.found = false
	}
	server.sequences[stream] = args.Sequence()

	batch := MiningBatch{Session: session, Endpoint: endpoint, Events: events, Closed: make(map[string]TapeCursor)}
	for key, path := range server.paths {
		if key.stream != stream {
			continue
		}
		batch.Closed[key.symbol] = path.cursors[path.excursion.ignitionIndex]
		if path.excursion.floor == 0 {
			batch.Flat = append(batch.Flat, key.symbol)
		}
	}
	server.batch, err = json.Marshal(batch)
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "mine: encode grading batch", err))
	}

	if len(events) == 0 {
		return nil
	}
	data, err := json.Marshal(events)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "mine: encode events", err))
	}
	server.out, err = json.Marshal(struct {
		ID       string `json:"capture_id"`
		Session  string `json:"capture_session"`
		Sequence int64  `json:"capture_sequence"`
		Endpoint string `json:"endpoint"`
		Channel  string `json:"channel"`
		Payload  []byte `json:"payload"`
		Count    int    `json:"event_count"`
	}{fmt.Sprintf("%s:%d", session, args.Sequence()), session, args.Sequence(), endpoint, channel, data, len(events)})

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "mine: encode archive row", err))
	}
	return nil
}

/* event samples A uniformly over the actual precursor observations, recording its seed. */
func (path *minedPath) event(symbol string, confirmed TapeCursor) (MinedEvent, error) {
	move := path.excursion.reported
	event := MinedEvent{Symbol: symbol, Precursor: path.cursors[move.anchorIndex], B: path.cursors[move.ignitionIndex], C: path.cursors[move.extremumIndex], D: confirmed, Excursion: move.excursion}
	eligible := move.ignitionIndex - move.anchorIndex

	if eligible == 0 {
		return event, nil
	}
	var seed [32]byte

	if _, err := rand.Read(seed[:]); err != nil {
		return event, errnie.Error(errnie.Err(errnie.Internal, "mine: sample precursor", err))
	}
	selected := random.New(random.NewChaCha8(seed)).IntN(eligible) + move.anchorIndex
	cursor := path.cursors[selected]
	event.A = &cursor
	event.Seed = hex.EncodeToString(seed[:])
	return event, nil
}

/* Done emits the batch of confirmed events once, suitable for IcebergTable. */
func (server *MineServer) Done(ctx context.Context, call Mine_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "mine: allocate result", err))
	}

	if err := results.SetBatch(server.batch); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "mine: batch", err))
	}

	if len(server.out) == 0 {
		results.SetNone()
		return nil
	}
	results.SetEvents()

	if err := results.Events().SetOut(server.out); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "mine: set events", err))
	}
	server.out = nil
	return nil
}

/* Compare orders records by numeric capture sequence and then frame position. */
func (cursor TapeCursor) Compare(other TapeCursor) int {
	if cursor.Sequence < other.Sequence {
		return -1
	}

	if cursor.Sequence > other.Sequence {
		return 1
	}

	if cursor.Record < other.Record {
		return -1
	}

	if cursor.Record > other.Record {
		return 1
	}
	return 0
}

/* MiningBatch supplies resolved truth separately from causal model inputs. */
type MiningBatch struct {
	Session  string
	Endpoint string
	Events   []MinedEvent
	Closed   map[string]TapeCursor
	Flat     []string
}
