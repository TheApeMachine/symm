package cognition

import (
	"context"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
	"strings"
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
			if err := keys.Set(0, `2:v1false7:BTC/USD:1:B1:A`); err != nil {
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
				if err := precursorInput(params, []string{fmt.Sprint(sequence)}, nil); err != nil {
					return err
				}
				input, err := params.Context()
				if err != nil {
					return err
				}
				input.SetSequence(int64(sequence))
				return nil
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
	input.SetEpoch(1)
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

func TestPrecursorWriteLifecycle(t *testing.T) {
	Convey("Inventory alternatives share the same causal token history", t, func() {
		model := store.Radix_ServerToClient(store.NewRadix())
		defer model.Release()
		So(model.Write(context.Background(), func(params store.Radix_write_Params) error {
			keys, err := params.NewKey(1)
			if err != nil {
				return err
			}
			if err := keys.Set(0, "2:v1false7:BTC/USD:1:C1:B1:A"); err != nil {
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
		send := func(epoch, sequence int64, holding bool, token string) string {
			So(client.Write(context.Background(), func(params Precursor_write_Params) error {
				if err := params.SetModel(model.AddRef()); err != nil {
					return err
				}
				if err := precursorInput(params, []string{token}, nil); err != nil {
					return err
				}
				input, err := params.Context()
				if err != nil {
					return err
				}
				input.SetEpoch(epoch)
				input.SetSequence(sequence)
				input.SetHolding(holding)
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			key, err := result.Ready().Key()
			So(err, ShouldBeNil)
			return key
		}
		for index, token := range []string{"A", "B", "C"} {
			flat := send(1, int64(index), false, token)
			held := send(1, int64(index), true, token)
			So(strings.Replace(flat, "false", "true", 1), ShouldEqual, held)
		}
		So(server.histories["BTC/USD"].steps, ShouldResemble, []string{"A", "B", "C"})
		So(send(1, 2, false, "C"), ShouldEndWith, "1:C1:B1:A")
		Convey("A new epoch cannot inherit the preceding run's path", func() {
			So(send(2, 0, false, "D"), ShouldEndWith, ":1:D")
			So(server.histories["BTC/USD"].steps, ShouldResemble, []string{"D"})
		})
		Convey("An older observation cannot rewrite an admitted live history", func() {
			So(client.Write(context.Background(), func(params Precursor_write_Params) error {
				if err := params.SetModel(model.AddRef()); err != nil {
					return err
				}
				return precursorInput(params, []string{"A"}, nil)
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}
