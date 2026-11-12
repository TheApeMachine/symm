package broker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
)

func TestDeskUpdateNotesTape(t *testing.T) {
	Convey("a trade's quote dollars land on the price tape", t, func() {
		price := &Price{Books: NewBookManager(), tape: NewTape()}
		desk := &Desk{price: price}
		at := time.Now()

		desk.Update([]*market.TradeData{{
			Symbol:    "AAA/USD",
			Price:     decimal.RequireFromString("10"),
			Qty:       decimal.RequireFromString("25"),
			Timestamp: at,
		}})

		recent, prior := price.TapeUptake("AAA/USD", at)
		So(recent, ShouldEqual, 250)
		So(prior, ShouldEqual, 0)
	})
}

func TestNewDeskLiveOnly(t *testing.T) {
	Convey("real model arms only the live tier", t, func() {
		prev := system.Cfg.Market.Model
		system.Cfg.Market.Model = "real"
		Reset(func() { system.Cfg.Market.Model = prev })

		// Nil private keeps Live disarmed; we only assert which tiers exist.
		desk := NewDesk(context.Background(), map[string]network.Client{
			"public": &deskTransportStub{},
		}, nil, nil)

		So(len(desk.tiers), ShouldEqual, 1)
		So(desk.tiers[TierTypeReal], ShouldNotBeNil)
		So(desk.tiers[TierTypeVirtual], ShouldBeNil)
		So(desk.tiers[TierTypePaper], ShouldBeNil)
	})
}

type deskTransportStub struct{}

func (*deskTransportStub) Read(...string) ([]byte, error) { return nil, nil }
func (*deskTransportStub) Write([]byte) error             { return nil }
func (*deskTransportStub) Post(string, any) ([]byte, error) {
	return nil, nil
}
func (*deskTransportStub) Watch(string, any, json.Unmarshaler) {}
