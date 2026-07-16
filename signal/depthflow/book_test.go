package depthflow

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBookOn(testingTB *testing.T) {
	Convey("Given a depthflow book ingestor", testingTB, func() {
		book := &Book{cache: bookCache()}
		payload := []byte(`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":10}],"asks":[{"price":101,"qty":10}],"checksum":1,"timestamp":"2023-09-25T09:04:31.742648Z"}]}`)

		Convey("When a book frame arrives", func() {
			book.On(payload)

			Convey("Then book rows should accumulate in cache", func() {
				So(len(bookRows(book.cache)), ShouldEqual, 1)
				So(bookRows(book.cache)[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})
}
