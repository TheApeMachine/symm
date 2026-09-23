package store

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type indexAnswer struct {
	Found    bool            `json:"found"`
	Position int             `json:"position"`
	Count    int             `json:"count"`
	Document json.RawMessage `json:"document"`
}

func indexStep(client Index, appends []string, requests []string) ([]indexAnswer, error) {
	ctx := context.Background()
	err := client.Write(ctx, func(params Index_write_Params) error {
		if err := params.SetPartition("capture.session,market.data.symbol"); err != nil {
			return err
		}

		if err := params.SetOrder("cursor.sequence,cursor.record"); err != nil {
			return err
		}
		list, err := params.NewAppend(int32(len(appends)))

		if err != nil {
			return err
		}

		for slot, document := range appends {
			if err := list.Set(slot, []byte(document)); err != nil {
				return err
			}
		}
		asks, err := params.NewRequest(int32(len(requests)))

		if err != nil {
			return err
		}

		for slot, request := range requests {
			if err := asks.Set(slot, []byte(request)); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
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
	list, err := results.Answers()

	if err != nil {
		return nil, err
	}
	answers := make([]indexAnswer, list.Len())

	for slot := range answers {
		raw, err := list.At(slot)

		if err != nil {
			return nil, err
		}

		if len(raw) == 0 {
			continue
		}

		if err := json.Unmarshal(bytes.Clone(raw), &answers[slot]); err != nil {
			return nil, err
		}
	}
	return answers, nil
}

func record(symbol, kind string, sequence int) string {
	return `{"capture":{"session":"s1"},"cursor":{"sequence":` + json.Number(itoa(sequence)).String() + `,"record":0},"market":{"channel":"level3","type":"` + kind + `","data":{"symbol":"` + symbol + `"}}}`
}

func itoa(value int) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func TestIndexWrite(t *testing.T) {
	Convey("Given a tape of two instruments streamed into the index", t, func() {
		client := Index_ServerToClient(NewIndex(context.Background()))
		defer client.Release()
		_, err := indexStep(client, []string{
			record("BTC/USD", "snapshot", 1), record("ETH/USD", "snapshot", 2), record("BTC/USD", "update", 3),
			record("BTC/USD", "snapshot", 5), record("BTC/USD", "update", 7), record("ETH/USD", "update", 8),
		}, nil)
		So(err, ShouldBeNil)

		Convey("Then each instrument is its own list in capture order", func() {
			answers, err := indexStep(client, nil, []string{
				`{"partition":["s1","BTC/USD"],"position":2}`,
				`{"partition":["s1","ETH/USD"],"position":1}`,
				`{"partition":["s1","BTC/USD"],"position":9}`,
			})
			So(err, ShouldBeNil)
			So(answers[0].Found, ShouldBeTrue)
			So(answers[0].Count, ShouldEqual, 4)
			So(string(answers[0].Document), ShouldContainSubstring, `"sequence":5`)
			So(string(answers[1].Document), ShouldContainSubstring, `"sequence":8`)
			So(answers[2].Found, ShouldBeFalse)
		})

		Convey("Then a seek finds a fragment's bounds and the snapshot its book starts from", func() {
			answers, err := indexStep(client, nil, []string{
				`{"partition":["s1","BTC/USD"],"seek":"first","from":{"cursor":{"sequence":4,"record":0}}}`,
				`{"partition":["s1","BTC/USD"],"seek":"last","from":{"cursor":{"sequence":4,"record":0}},"where":{"market.type":"snapshot"}}`,
				`{"partition":["s1","BTC/USD"],"seek":"last","from":{"cursor":{"sequence":7,"record":0}},"where":{"market.type":"snapshot"}}`,
				`{"partition":["s1","BTC/USD"],"seek":"first","from":{"cursor":{"sequence":8,"record":0}}}`,
			})
			So(err, ShouldBeNil)
			So(answers[0].Position, ShouldEqual, 2)
			So(answers[1].Position, ShouldEqual, 0)
			So(answers[2].Position, ShouldEqual, 2)
			So(answers[3].Found, ShouldBeFalse)
		})

		Convey("Then an evaluation with nothing arriving answers nothing", func() {
			answers, err := indexStep(client, nil, nil)
			So(err, ShouldBeNil)
			So(answers, ShouldBeEmpty)
		})

		Convey("Then another capture session is its own partition, so its sequence may restart", func() {
			_, err := indexStep(client, []string{strings.Replace(record("BTC/USD", "snapshot", 1), `"s1"`, `"s2"`, 1)}, nil)
			So(err, ShouldBeNil)
			answers, err := indexStep(client, nil, []string{`{"partition":["s2","BTC/USD"],"position":0}`})
			So(err, ShouldBeNil)
			So(answers[0].Count, ShouldEqual, 1)
		})

		Convey("Then a document appended out of capture order is refused", func() {
			_, err := indexStep(client, []string{record("BTC/USD", "update", 6)}, nil)
			So(err, ShouldNotBeNil)
		})
	})
}
