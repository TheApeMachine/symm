package broker

import (
	"context"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
)

/*
History persists open lots so a restart can exit what the previous process
entered, and drops them when the paper wallet resets.
*/
type History struct {
	*runtime.System
	price          *Price
	papers         *Cache
	reals          *Cache
	virtuals       *Cache
	store          *store.Client
	paperTransport Transport
	liveTransport  Transport
	onSettled      func(*Position)
}

/*
lotRecord is one stored lot: clock fields plus the position object.
*/
type lotRecord struct {
	Epoch     uint64    `json:"epoch"`
	Tick      uint64    `json:"tick"`
	Timestamp uint64    `json:"timestamp"`
	Position  *Position `json:"position"`
}

func NewHistory(
	ctx context.Context,
	price *Price,
	papers *Cache,
	reals *Cache,
	virtuals *Cache,
	paperTransport Transport,
	liveTransport Transport,
	onSettled func(*Position),
	rails *AdaptiveRails,
) *History {
	history := &History{
		System:         runtime.NewSystem(ctx, "history"),
		price:          price,
		papers:         papers,
		reals:          reals,
		virtuals:       virtuals,
		paperTransport: paperTransport,
		liveTransport:  liveTransport,
		store:          store.NewClient(ctx, system.Cfg.Storage.S3.Bucket),
		onSettled:      onSettled,
	}

	for key, cache := range map[string]*Cache{
		"paper":   papers,
		"real":    reals,
		"virtual": virtuals,
	} {
		transport := paperTransport
		if key == "real" && liveTransport != nil {
			transport = liveTransport
		}

		for _, state := range []string{"open", "closed"} {
			for raw := range history.store.Stream(
				history.Context(), fmt.Sprintf("%s/%s", key, state),
			) {
				position := history.restore(raw)

				if position == nil {
					continue
				}

				position.price = price
				position.transport = transport
				position.Structural = NewStructuralStop()
				position.Learning = NewLearningStop(rails)
				position.onSettled = onSettled

				if position.Book == "" {
					position.Book = ParseLedger(key, key == "virtual").String()
					position.Virtual = position.Book == string(LedgerVirtual)
				}

				cache.Add(position.Symbol, position, state == "open")
			}
		}
	}

	return history
}

/*
restore reads one lot from a stored JSON object.
*/
func (history *History) restore(raw []byte) *Position {
	if len(raw) == 0 {
		return nil
	}

	position, err := data.AttributeOk[Position](raw, "position")

	if err != nil || position.Symbol == "" {
		return nil
	}

	return &position
}

/*
Store writes one lot at key as JSON {"epoch","tick","timestamp","position"}.
*/
func (history *History) Store(key string, position *Position) error {
	if key == "" || position == nil {
		return nil
	}

	record := lotRecord{
		Epoch:     data.Epoch.Load(),
		Tick:      data.Tick.Load(),
		Timestamp: uint64(time.Now().UnixNano()),
		Position:  position,
	}

	body, err := sonic.Marshal(record)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[history.Store] failed to encode lot",
			err,
		))
	}

	if _, err = history.store.WriteAtPrefix(key, body); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[history.Store] failed to store lot",
			err,
		))
	}

	return nil
}

func (history *History) Drop(prefix string) error {
	if prefix == "" {
		return nil
	}

	return history.store.DeleteOtherKeys(prefix, "")
}

/*
Move retires the open records of a lot and stores the closed one.
*/
func (history *History) Move(key string, position *Position, from string) error {
	if key == "" || position == nil {
		return nil
	}

	if err := history.Store(key, position); err != nil {
		return err
	}

	if from == "" {
		return nil
	}

	if err := history.store.DeletePrefix(from); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[history.Move] failed to retire open lot",
			err,
		))
	}

	return nil
}
