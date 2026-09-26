package cognition

import (
	"context"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
	"testing"
)

func TestPrecursorWrite(t *testing.T) {
	Convey("Live token retention is bounded by actual model representation", t, func() {
		model := store.Radix_ServerToClient(store.NewRadix())
		defer model.Release()
		So(model.Write(context.Background(), func(params store.Radix_write_Params) error {
			keys, err := params.NewKey(1)
			if err != nil {
				return err
			}
			if err := keys.Set(0, `["v1",false,"BTC/USD"]:1:B1:A`); err != nil {
				return err
			}
			values, err := params.NewValue(1)
			if err != nil {
				return err
			}
			return values.Set(0, []byte(`{"ENTER":1}`))
		}), ShouldBeNil)
		So(model.WaitStreaming(), ShouldBeNil)
		server := NewPrecursor()
		client := Precursor_ServerToClient(server)
		defer client.Release()
		for sequence := range 128 {
			So(client.Write(context.Background(), func(params Precursor_write_Params) error {
				if err := params.SetModel(model.AddRef()); err != nil {
					return err
				}
				return precursorInput(params, []string{fmt.Sprint(sequence)}, nil)
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Ready().Prefix(), ShouldBeTrue)
			release()
		}
		So(len(server.histories["BTC/USD"].steps), ShouldEqual, server.extent)
		So(len(server.histories), ShouldEqual, 1)
	})
}

func BenchmarkPrecursorWrite(b *testing.B) {
	model := store.Radix_ServerToClient(store.NewRadix())
	defer model.Release()
	client := Precursor_ServerToClient(NewPrecursor())
	defer client.Release()
	b.ReportAllocs()
	for b.Loop() {
		if err := client.Write(context.Background(), func(params Precursor_write_Params) error {
			if err := params.SetModel(model.AddRef()); err != nil {
				return err
			}
			return precursorInput(params, []string{"A", "B"}, []string{"A", "B", "C", "A", "B"})
		}); err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(context.Background(), nil)
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}

/* precursorInput builds typed evidence for the real node boundary. */
func precursorInput(params Precursor_write_Params, tokens, history []string) error {
	input, err := params.NewContext()
	if err != nil {
		return err
	}
	if err := input.SetSymbol("BTC/USD"); err != nil {
		return err
	}
	if err := input.SetVocabulary("v1"); err != nil {
		return err
	}
	values, err := input.NewTokens(int32(len(tokens)))
	if err != nil {
		return err
	}
	for index, value := range tokens {
		if err := values.Set(index, value); err != nil {
			return err
		}
	}
	input.SetLive()
	if history == nil {
		return nil
	}
	steps, err := input.NewHistory(int32(len(history)))
	if err != nil {
		return err
	}
	for index, value := range history {
		if err := steps.Set(index, value); err != nil {
			return err
		}
	}
	return nil
}
