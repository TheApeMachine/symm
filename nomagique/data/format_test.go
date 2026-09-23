package data

import (
	"bytes"
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func format(template string, values ...[]byte) ([]byte, error) {
	ctx := context.Background()
	client := Format_ServerToClient(NewFormat(context.Background()))
	defer client.Release()

	if err := client.Write(ctx, func(params Format_write_Params) error {
		list, err := params.NewValues(int32(len(values)))

		if err != nil {
			return err
		}

		for index, value := range values {
			if err := list.Set(index, value); err != nil {
				return err
			}
		}
		return params.SetTemplate(template)
	}); err != nil {
		return nil, err
	}

	if err := client.WaitStreaming(); err != nil {
		return nil, err
	}
	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		return nil, err
	}
	if results.Which() == Formatted_Which_idle {
		return nil, nil
	}
	out, err := results.Out()
	return bytes.Clone(out), err
}

func TestFormatWrite(t *testing.T) {
	Convey("Given a template mixing literal text and computed bytes", t, func() {
		digest := []byte{0x00, 0xff, '{', 0x7b}

		Convey("Then each placeholder carries its value's raw bytes, in any order and repeated", func() {
			out, err := format("/0/private/Token{1}|{0}{0}", []byte("n=1"), digest)
			So(err, ShouldBeNil)
			So(out, ShouldResemble, append(append([]byte("/0/private/Token"), digest...), []byte("|n=1n=1")...))
		})

		Convey("Then doubled braces are literal", func() {
			out, err := format(`{{"nonce":{0}}}`, []byte("42"))
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, `{"nonce":42}`)
		})

		Convey("Then a placeholder naming no value is an error, not an empty substitution", func() {
			_, err := format("API-Key: {1}", []byte("only one"))
			So(err, ShouldNotBeNil)
		})

		Convey("Then nothing arriving at all leaves it idle", func() {
			out, err := format("API-Sign: {0}", []byte{})
			So(err, ShouldBeNil)
			So(out, ShouldBeNil)
		})

		Convey("Then a value that did not arrive beside one that did is an error", func() {
			_, err := format("{0} {1}", []byte("arrived"), []byte{})
			So(err, ShouldNotBeNil)
		})

		Convey("Then an unbalanced brace is an error", func() {
			_, err := format("broken {0", []byte("x"))
			So(err, ShouldNotBeNil)
		})
	})
}
