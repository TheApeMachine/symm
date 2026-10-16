package network

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/data/transport"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
)

var (
	replayer *Replayer
	once     sync.Once
)

/*
Replayer wraps a transport.Stream connected to the shared replayerDriver.
*/
type Replayer struct {
	*runtime.System
	store   *store.Client
	streams map[string]*transport.Stream
}

/*
NewReplayer returns a Replayer for the given stream key (e.g. "public", "private").
All Replayers share the same underlying driver via sync.Once, ensuring synchronized ticks.
*/
func NewReplayer(ctx context.Context, key string) *Replayer {
	once.Do(func() {
		replayer = &Replayer{
			System: runtime.NewSystem(ctx, "replay"),
			store:  store.NewClient(ctx, key),
			streams: map[string]*transport.Stream{
				"public":  transport.NewStream(ctx, nil),
				"private": transport.NewStream(ctx, nil),
			},
		}
	})

	return replayer
}

func (replayer *Replayer) Read(p []byte) (n int, err error) {
	value := data.From(p)

	switch value.Origin {
	case "spot:trade", "spot:instrument":
		_, err = io.Copy(value, replayer.streams["public"])
	case "spot:level3", "spot:executions", "spot:orders":
		_, err = io.Copy(value, replayer.streams["private"])
	}

	if err != nil {
		return 0, err
	}

	return value.Read(p)
}

func (replayer *Replayer) Write(p []byte) (n int, err error) {
	value := data.From(p)

	switch value.Origin {
	case "spot:trade", "spot:instrument":
		_, err = io.Copy(replayer.streams["public"], value)
	case "spot:level3", "spot:executions", "spot:orders":
		_, err = io.Copy(replayer.streams["private"], value)
	}

	if err != nil {
		return 0, err
	}

	return len(p), nil
}

func (replayer *Replayer) Close() error {
	return replayer.store.Close()
}
