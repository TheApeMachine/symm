package cmd

import (
	"context"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	nmruntime "github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/strategy"
)

func TestBooterInitTransports(t *testing.T) {
	Convey("initTransports arms virtual, paper, and websocket clients in paper mode", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		booter := &Booter{System: nmruntime.NewSystem(ctx, "test")}

		So(booter.initTransports(), ShouldBeNil)
		So(booter.virtual, ShouldNotBeNil)
		So(booter.paper, ShouldNotBeNil)
		So(booter.public, ShouldNotBeNil)
		So(booter.private, ShouldNotBeNil)
	})
}

func TestBooterRunRejectsMissingCredentials(t *testing.T) {
	Convey("Run returns credential errors without touching the network", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		booter := &Booter{
			System:  nmruntime.NewSystem(ctx, "test"),
			credErr: errors.New("kraken credentials unavailable"),
		}

		So(booter.Run(), ShouldNotBeNil)
	})
}

func TestBooterShutdownWithoutPipeline(t *testing.T) {
	Convey("shutdown completes when desk and training were never started", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		booter := &Booter{System: nmruntime.NewSystem(ctx, "test")}

		So(booter.shutdown(), ShouldBeNil)
	})
}

func TestBooterShutdownFlushesPipeline(t *testing.T) {
	Convey("shutdown runs desk and training flush hooks when wired", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		symbol := "BOOT/USD"
		desk, price := broker.PaperBook(ctx, symbol, nil)
		training := strategy.NewTraining(ctx, price, desk, nil)

		booter := &Booter{
			System:   nmruntime.NewSystem(ctx, "test"),
			desk:     desk,
			training: training,
		}

		So(booter.shutdown(), ShouldBeNil)
	})
}
