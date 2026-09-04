package store

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestKeyStore(t *testing.T) {
	Convey("Given a KeyStore with a dynamic key selector", t, func() {
		activeKey := "BTC/USD"
		keyStore := NewKeyStore(func() string { return activeKey })

		Convey("retrieving the active store initializes a fresh store for that key", func() {
			firstStore := keyStore.Active()

			So(firstStore, ShouldNotBeNil)
			So(keyStore.Windows, ShouldContainKey, "BTC/USD")

			Convey("switching the key retrieves a distinct store", func() {
				activeKey = "ETH/USD"
				secondStore := keyStore.Active()

				So(secondStore, ShouldNotBeNil)
				So(secondStore, ShouldNotPointTo, firstStore)
				So(len(keyStore.Windows), ShouldEqual, 2)
			})
		})
	})
}
