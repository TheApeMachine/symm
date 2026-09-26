package cognition

import (
	"context"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestContextBuilderWrite(t *testing.T) {
	Convey("Native contexts preserve values and separate inventory state", t, func() {
		client := ContextBuilder_ServerToClient(NewContextBuilder())
		defer client.Release()
		for _, replay := range []bool{false, true, false} {
			So(client.Write(context.Background(), func(args ContextBuilder_write_Params) error {
				args.SetReplay(replay)
				args.SetEpoch(1790412345678901234)
				args.SetSequence(9007199254740993)
				if err := args.SetSymbol("BTC/USD"); err != nil {
					return err
				}
				if err := args.SetVocabulary("region:[]\"/identity"); err != nil {
					return err
				}
				tokens, err := args.NewTokens(2)
				if err != nil {
					return err
				}
				for index, value := range []string{"12", "34"} {
					if err := tokens.Set(index, value); err != nil {
						return err
					}
				}
				if !replay {
					return nil
				}
				history, err := args.NewHistory(2)
				if err != nil {
					return err
				}
				if err := history.Set(0, "56"); err != nil {
					return err
				}
				return history.Set(1, "12,34")
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Which(), ShouldEqual, Contexts_Which_ready)
			flat, err := result.Ready().Flat()
			So(err, ShouldBeNil)
			held, err := result.Ready().Held()
			So(err, ShouldBeNil)
			So(flat.Holding(), ShouldBeFalse)
			So(held.Holding(), ShouldBeTrue)
			for _, input := range []Context{flat, held} {
				So(input.Epoch(), ShouldEqual, int64(1790412345678901234))
				So(input.Sequence(), ShouldEqual, int64(9007199254740993))
				vocabulary, err := input.Vocabulary()
				So(err, ShouldBeNil)
				So(vocabulary, ShouldEqual, "region:[]\"/identity")
				tokens, err := input.Tokens()
				So(err, ShouldBeNil)
				So(tokens.Len(), ShouldEqual, 2)
				So(input.Which() == Context_Which_history, ShouldEqual, replay)
			}
			release()
		}
		Convey("No active tokens produce no fabricated context", func() {
			So(client.Write(context.Background(), nil), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			So(result.Which(), ShouldEqual, Contexts_Which_idle)
		})
	})
}

func BenchmarkContextBuilderWrite(b *testing.B) {
	client := ContextBuilder_ServerToClient(NewContextBuilder())
	defer client.Release()
	b.ReportAllocs()
	for b.Loop() {
		if err := client.Write(context.Background(), func(args ContextBuilder_write_Params) error {
			args.SetEpoch(1)
			if err := args.SetSymbol("BTC/USD"); err != nil {
				return err
			}
			if err := args.SetVocabulary("partition"); err != nil {
				return err
			}
			tokens, err := args.NewTokens(2)
			if err != nil {
				return err
			}
			if err := tokens.Set(0, "17"); err != nil {
				return err
			}
			return tokens.Set(1, "39")
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
