package cmd

import (
	"context"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/store/storetest"
)

func TestUploadPaper(t *testing.T) {
	Convey("Given an in-memory bucket and two paper records", t, func() {
		backend := storetest.NewBackend()
		server := httptest.NewServer(backend)
		defer server.Close()

		client := store.NewClient(context.Background(), "symm", storetest.Options(server.URL))
		So(client, ShouldNotBeNil)

		record := func(action string, seq uint64) *data.Value {
			value := data.NewValue(data.PaperOrigin, action, "BTC/USD")
			value.Epoch, value.Tick = 7, seq
			value.FormatPrefix()
			So(data.AppendPaper(value, "R01/R04", 3, make([]float64, len(data.PaperKeys))), ShouldBeNil)
			return value
		}

		records := []*data.Value{record("enter", 1), record("exit", 2)}

		Convey("It uploads each under its own key", func() {
			So(uploadPaper(client, records), ShouldBeNil)
			So(backend.Objects, ShouldContainKey, records[0].Prefix)
			So(backend.Objects, ShouldContainKey, records[1].Prefix)
		})

		Convey("A key already stored refuses the whole upload, naming it, and writes nothing", func() {
			backend.Objects[records[1].Prefix] = []byte("stored before")

			err := uploadPaper(client, records)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, records[1].Prefix)
			So(backend.Objects, ShouldNotContainKey, records[0].Prefix)
			So(string(backend.Objects[records[1].Prefix]), ShouldEqual, "stored before")
		})

		Convey("A key repeating in the file is refused", func() {
			err := uploadPaper(client, []*data.Value{records[0], records[0]})
			So(err, ShouldNotBeNil)
			So(backend.Objects, ShouldBeEmpty)
		})
	})
}
