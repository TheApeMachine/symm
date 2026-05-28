package wallet

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestWalletPut(t *testing.T) {
	convey.Convey("Given a wallet", t, func() {
		wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
		convey.Convey("It should put cash into the wallet", func() {
			err := wallet.Put(10)
			convey.So(err, convey.ShouldBeNil)
			convey.So(wallet.Balance, convey.ShouldEqual, 190)
			convey.So(wallet.ReservedEUR, convey.ShouldEqual, 10)
		})
	})
}

func TestWalletTake(t *testing.T) {
	convey.Convey("Given a wallet with reserved cash", t, func() {
		wallet := NewWallet(PaperWallet, "EUR", 190, 0.26)
		wallet.ReservedEUR = 10
		convey.Convey("It should return reserved cash to balance", func() {
			err := wallet.Take(10)
			convey.So(err, convey.ShouldBeNil)
			convey.So(wallet.Balance, convey.ShouldEqual, 200)
			convey.So(wallet.ReservedEUR, convey.ShouldEqual, 0)
		})
	})
}

func TestWalletReserve(t *testing.T) {
	convey.Convey("Given a wallet", t, func() {
		wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
		convey.Convey("It should reserve cash from the wallet", func() {
			err := wallet.Reserve(10)
			convey.So(err, convey.ShouldBeNil)
			convey.So(wallet.Balance, convey.ShouldEqual, 190)
			convey.So(wallet.ReservedEUR, convey.ShouldEqual, 10)
		})
		convey.Convey("It should reject reservations larger than the balance", func() {
			err := wallet.Reserve(250)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(wallet.Balance, convey.ShouldEqual, 200)
			convey.So(wallet.ReservedEUR, convey.ShouldEqual, 0)
		})
	})
}

func BenchmarkWalletPut(b *testing.B) {
	wallet := NewWallet(PaperWallet, "EUR", 1e12, 0.26)

	for i := 0; i < b.N; i++ {
		wallet.Put(10)
	}
}

func BenchmarkWalletTake(b *testing.B) {
	wallet := NewWallet(PaperWallet, "EUR", 200, 0.26)
	wallet.ReservedEUR = 10

	for i := 0; i < b.N; i++ {
		wallet.Take(10)
	}
}

func BenchmarkWalletReserve(b *testing.B) {
	wallet := NewWallet(PaperWallet, "EUR", 1e12, 0.26)

	for i := 0; i < b.N; i++ {
		wallet.Reserve(10)
	}
}
