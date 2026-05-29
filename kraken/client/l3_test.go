package client

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
)

func TestNewL3Client(t *testing.T) {
	Convey("Given API credentials", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		l3 := NewL3Client(ctx, pool, "wss://ws-auth.kraken.com/v2", "key", "secret")

		Convey("It should wire level3 and subscription groups", func() {
			So(l3, ShouldNotBeNil)
			So(l3.broadcasts["level3"], ShouldNotBeNil)
			So(l3.subscribers["subscriptions"], ShouldNotBeNil)
			So(l3.State(), ShouldEqual, engine.READY)
			So(l3.Start(), ShouldBeNil)
		})
	})
}

func TestL3MarkRequested(t *testing.T) {
	Convey("Given an L3 client subscription set", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		l3 := NewL3Client(ctx, pool, "wss://example", "key", "secret")

		fresh := l3.markRequested([]string{"BTC/EUR", "ETH/EUR", "BTC/EUR", ""})
		again := l3.markRequested([]string{"BTC/EUR", "SOL/EUR"})

		Convey("It should dedupe and skip empty symbols", func() {
			So(fresh, ShouldResemble, []string{"BTC/EUR", "ETH/EUR"})
			So(again, ShouldResemble, []string{"SOL/EUR"})
		})
	})
}

func TestL3ConnectOnceRequiresCredentials(t *testing.T) {
	Convey("Given an L3 client without credentials", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		l3 := NewL3Client(ctx, pool, "wss://example", "", "")

		Convey("It should reject connect", func() {
			So(l3.connectOnce(), ShouldNotBeNil)
		})
	})
}

func BenchmarkL3MarkRequested(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	l3 := NewL3Client(ctx, pool, "wss://example", "key", "secret")
	symbols := []string{"BTC/EUR", "ETH/EUR", "SOL/EUR"}

	for b.Loop() {
		l3.markRequested(symbols)
	}
}
