package temporal

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
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
	batches   []MiningBatch
	out       [][]byte
	batch     [][]byte
}

func NewMine() *MineServer {
	return &MineServer{paths: make(map[miningSymbol]*minedPath), sequences: make(map[miningStream]int64)}
}

/*
Write mines the archived frames of one session handed over together, in
capture order, as if each had arrived alone.
*/
func (server *MineServer) Write(ctx context.Context, call Mine_write) error {
	server.out = server.out[:0]
	server.batch = server.batch[:0]
	args := call.Args()
	call.Args().Message().ResetReadLimit(math.MaxUint64)
	payloads, err := args.Payload()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: payload", err))
	}
	sequences, err := args.Sequence()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: sequence", err))
	}
	endpoints, err := args.Endpoint()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: endpoint", err))
	}
	session, err := args.Session()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: session", err))
	}
	channel, err := args.Channel()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: channel", err))
	}
	priceField, err := args.PriceField()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "mine: price field", err))
	}

	if payloads.Len() != sequences.Len() || payloads.Len() != endpoints.Len() {
		return errnie.Error(errnie.Err(errnie.Validation, fmt.Sprintf(
			"mine: %d payloads, %d sequences and %d endpoints", payloads.Len(), sequences.Len(), endpoints.Len(),
		), nil))
	}

	batches := make(map[miningStream]int)

	for frame := range payloads.Len() {
		payload, err := payloads.At(frame)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "mine: payload", err))
		}
		endpoint, err := endpoints.At(frame)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "mine: endpoint", err))
		}
		stream := miningStream{session, endpoint, channel}
		events, err := server.mine(ctx, payload, stream, sequences.At(frame), priceField)

		if err != nil {
			return err
		}

		if len(events) == 0 {
			continue
		}

		if err := server.archive(stream, sequences.At(frame), events); err != nil {
			return err
		}

		position, found := batches[stream]

		if !found {
			position = len(server.batches)
			batches[stream] = position
			server.batches = append(server.batches, MiningBatch{Session: session, Endpoint: endpoint})
		}

		server.batches[position].Events = append(server.batches[position].Events, events...)
	}

	for _, batch := range server.batches {
		encoded, err := json.Marshal(batch)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "mine: encode grading batch", err))
		}

		server.batch = append(server.batch, encoded)
	}

	server.batches = server.batches[:0]
	return nil
}

/*
mine steps every instrument one frame carries and returns the moves that
frame confirmed.
*/
func (server *MineServer) mine(
	ctx context.Context, payload []byte, stream miningStream, sequence int64, priceField string,
) ([]MinedEvent, error) {
	if stream.session == "" || stream.endpoint == "" || stream.channel == "" || priceField == "" || sequence < 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: session, endpoint, channel, price field and sequence are required", nil))
	}

	var envelope struct {
		Channel string          `json:"channel"`
		Data    json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: decode frame", err))
	}

	// Other channels shape their data as they like; only the mined one is read.
	if envelope.Channel != stream.channel {
		return nil, nil
	}
	var records []map[string]json.RawMessage

	if err := json.Unmarshal(envelope.Data, &records); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: decode "+stream.channel+" data", err))
	}

	if previous, found := server.sequences[stream]; found && sequence <= previous {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "mine: capture sequence did not advance", nil))
	}

	// Validate the entire frame before changing any instrument state.
	readings, err := readPrices(records, priceField)

	if err != nil {
		return nil, err
	}
	events := make([]MinedEvent, 0)

	for _, reading := range readings {
		key := miningSymbol{stream, reading.symbol}
		path := server.paths[key]

		if path == nil {
			path = &minedPath{excursion: NewExcursion(ctx)}
			server.paths[key] = path
		}
		cursor := TapeCursor{sequence, reading.record}
		path.cursors = append(path.cursors, cursor)

		if err := path.excursion.Step(reading.value); err != nil {
			return nil, err
		}

		if !path.excursion.reported.found {
			continue
		}
		event, err := path.event(reading.symbol, cursor)

		if err != nil {
			return nil, err
		}
		events = append(events, event)
		path.excursion.reported.found = false
	}
	server.sequences[stream] = sequence
	return events, nil
}

/*
archive adds the row that records one frame's confirmed moves.
*/
func (server *MineServer) archive(stream miningStream, sequence int64, events []MinedEvent) error {
	data, err := json.Marshal(events)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "mine: encode events", err))
	}
	row, err := json.Marshal(struct {
		ID       string `json:"capture_id"`
		Session  string `json:"capture_session"`
		Sequence int64  `json:"capture_sequence"`
		Endpoint string `json:"endpoint"`
		Channel  string `json:"channel"`
		Payload  []byte `json:"payload"`
		Count    int    `json:"event_count"`
	}{fmt.Sprintf("%s:%d", stream.session, sequence), stream.session, sequence, stream.endpoint, stream.channel, data, len(events)})

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "mine: encode archive row", err))
	}

	server.out = append(server.out, row)
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

/* Done hands over what the frames just mined confirmed, once. */
func (server *MineServer) Done(ctx context.Context, call Mine_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "mine: allocate result", err))
	}

	if len(server.out) == 0 {
		results.SetNone()
		return nil
	}

	results.SetEvents()
	events := results.Events()

	for _, set := range []struct {
		name  string
		items [][]byte
		alloc func(int32) (capnp.DataList, error)
	}{
		{"batch", server.batch, events.NewBatch},
		{"out", server.out, events.NewOut},
	} {
		list, err := set.alloc(int32(len(set.items)))

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "mine: allocate "+set.name, err))
		}

		for index, item := range set.items {
			if err := list.Set(index, item); err != nil {
				return errnie.Error(errnie.Err(errnie.Internal, "mine: set "+set.name, err))
			}
		}
	}

	server.out, server.batch = server.out[:0], server.batch[:0]
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
}
