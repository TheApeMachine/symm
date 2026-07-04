package trader

import (
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
)

type History[T any] struct {
	cache *sync.Map
	depth int
}

func NewHistory[T any]() *History[T] {
	return &History[T]{
		cache: &sync.Map{},
		depth: viper.GetViper().GetInt("signals.feed_ring_capacity"),
	}
}

func (history *History[T]) Measure(symbol string, wall time.Time, payload T) error {
	if history.depth <= 0 {
		return errnie.Err(
			errnie.Validation,
			"trader: signals.feed_ring_capacity must be positive",
			nil,
		)
	}

	if strings.TrimSpace(symbol) == "" {
		return errnie.Err(errnie.Validation, "trader: symbol required", nil)
	}

	if wall.IsZero() {
		return errnie.Err(errnie.Validation, "trader: timestamp required", nil)
	}

	ring, _ := history.cache.LoadOrStore(
		symbol, structure.NewClockRing[T](
			history.depth,
			history.depth,
			history.depth,
		),
	)

	clock := ring.(*structure.ClockRing[T])

	clock.Push(structure.ClockSlot[T]{
		Wall:    wall,
		Payload: payload,
	})

	return nil
}
