package store

import (
	"context"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store/storetest"
)

func TestReader(t *testing.T) {
	Convey("Given an in-memory bucket", t, func() {
		ctx := context.Background()
		backend := storetest.NewBackend()
		server := httptest.NewServer(backend)
		defer server.Close()

		Convey("A bucket that does not exist is unreachable: an error", func() {
			_, err := NewReader(ctx, "symm", storetest.Options(server.URL))
			So(err, ShouldNotBeNil)
			// The read-only view never creates it.
			So(backend.Buckets, ShouldBeEmpty)
		})

		Convey("It lists epochs and decodes Values, and reports what it cannot read", func() {
			backend.Buckets["symm"] = true

			for _, epoch := range []uint64{3, 9} {
				value := &data.Value{Origin: "policy", Role: "enter", Scope: "all", Epoch: epoch, Tick: 1}
				value.FormatPrefix()
				backend.Objects[value.Prefix] = value.Bytes()
			}

			reader, err := NewReader(ctx, "symm", storetest.Options(server.URL))
			So(err, ShouldBeNil)

			epochs, err := reader.Epochs("policy/enter/all/")
			So(err, ShouldBeNil)
			So(epochs, ShouldResemble, []uint64{3, 9})

			collect := func() ([]*data.Value, error) {
				values := make([]*data.Value, 0)

				for value, err := range reader.Values("policy/enter/all/") {
					if err != nil {
						return values, err
					}

					values = append(values, value)
				}

				return values, nil
			}

			values, err := collect()
			So(err, ShouldBeNil)
			So(values, ShouldHaveLength, 2)
			So(values[1].Epoch, ShouldEqual, 9)

			garbage := "policy/enter/all/0000000000000000009/0000000000000000002.bin"
			backend.Objects[garbage] = []byte("garbage")
			values, err = collect()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, garbage)
			// The two Values before it were yielded; the bad key ended it.
			So(values, ShouldHaveLength, 2)

			backend.Objects["policy/enter/all/not-an-epoch/x.bin"] = []byte("x")
			_, err = reader.Epochs("policy/enter/all/")
			So(err, ShouldNotBeNil)

			So(backend.Objects, ShouldHaveLength, 4)
		})

		Convey("Its method set is read-only: listing and reading, nothing that writes", func() {
			methods := make([]string, 0)
			readerType := reflect.TypeFor[*Reader]()

			for idx := range readerType.NumMethod() {
				methods = append(methods, readerType.Method(idx).Name)
			}

			slices.Sort(methods)
			So(methods, ShouldResemble, []string{"Epochs", "PolicyGates", "Values"})
		})
	})
}

func TestValues(t *testing.T) {
	Convey("Given a client over an in-memory bucket", t, func() {
		backend := storetest.NewBackend()
		server := httptest.NewServer(backend)
		defer server.Close()

		client := NewClient(context.Background(), "symm", storetest.Options(server.URL))

		for tick := uint64(1); tick <= 2; tick++ {
			value := &data.Value{Origin: "policy", Role: "enter", Scope: "all", Epoch: 3, Tick: tick}
			value.FormatPrefix()
			backend.Objects[value.Prefix] = value.Bytes()
		}

		read := func() (int, error) {
			count := 0

			for _, err := range Values(client, "policy/enter/all/") {
				if err != nil {
					return count, err
				}

				count++
			}

			return count, nil
		}

		Convey("It yields every Value in key order", func() {
			count, err := read()
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 2)
		})

		Convey("A key that does not decode is an error naming it, not a skip", func() {
			key := "policy/enter/all/0000000000000000003/0000000000000000009.bin"
			backend.Objects[key] = []byte("garbage")

			count, err := read()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, key)
			So(count, ShouldEqual, 2)
		})

		Convey("No client is an error", func() {
			for _, err := range Values(nil, "policy/") {
				So(err, ShouldNotBeNil)
			}
		})
	})
}
