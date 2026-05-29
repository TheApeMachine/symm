package wallet

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWalletReserveEntry(t *testing.T) {
	Convey("Given entry reservation flows", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 200, 0.26)

		err := tradingWallet.ReserveEntry(50)

		Convey("It should lock cash for a pending entry", func() {
			So(err, ShouldBeNil)
			So(tradingWallet.Balance, ShouldAlmostEqual, 150, 1e-9)
			So(tradingWallet.ReservedEUR, ShouldAlmostEqual, 50, 1e-9)
		})

		Convey("It should reject reservations above available cash", func() {
			err := tradingWallet.ReserveEntry(300)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestWalletReleaseEntryReservation(t *testing.T) {
	Convey("Given reserved entry cash", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 150, 0.26)
		tradingWallet.ReservedEUR = 50

		tradingWallet.ReleaseEntryReservation(50)

		Convey("It should return reserved cash to balance", func() {
			So(tradingWallet.Balance, ShouldAlmostEqual, 200, 1e-9)
			So(tradingWallet.ReservedEUR, ShouldEqual, 0)
		})
	})
}

func TestWalletSettleEntryReservation(t *testing.T) {
	Convey("Given a reserved entry", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 150, 0.26)
		tradingWallet.ReservedEUR = 50

		Convey("It should spend reserved cash when actual equals reserved", func() {
			err := tradingWallet.SettleEntryReservation(50, 50)
			So(err, ShouldBeNil)
			So(tradingWallet.Balance, ShouldAlmostEqual, 150, 1e-9)
			So(tradingWallet.ReservedEUR, ShouldEqual, 0)
		})

		Convey("It should debit extra cash when fill exceeds reservation", func() {
			tradingWallet.ReservedEUR = 50
			err := tradingWallet.SettleEntryReservation(50, 60)
			So(err, ShouldBeNil)
			So(tradingWallet.Balance, ShouldAlmostEqual, 140, 1e-9)
		})

		Convey("It should refund unused reservation when fill is smaller", func() {
			tradingWallet.ReservedEUR = 50
			err := tradingWallet.SettleEntryReservation(50, 40)
			So(err, ShouldBeNil)
			So(tradingWallet.Balance, ShouldAlmostEqual, 160, 1e-9)
			So(tradingWallet.ReservedEUR, ShouldEqual, 0)
		})
	})

	Convey("Given no prior reservation", t, func() {
		tradingWallet := NewWallet(PaperWallet, "EUR", 100, 0.26)

		Convey("It should debit balance directly for the fill cost", func() {
			err := tradingWallet.SettleEntryReservation(0, 25)
			So(err, ShouldBeNil)
			So(tradingWallet.Balance, ShouldAlmostEqual, 75, 1e-9)
		})

		Convey("It should reject fills that exceed available cash", func() {
			err := tradingWallet.SettleEntryReservation(0, 150)
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkWalletSettleEntryReservation(b *testing.B) {
	tradingWallet := NewWallet(PaperWallet, "EUR", 1e9, 0.26)

	for b.Loop() {
		tradingWallet.ReservedEUR = 50
		tradingWallet.Balance = 1e9 - 50
		_ = tradingWallet.SettleEntryReservation(50, 48)
	}
}
