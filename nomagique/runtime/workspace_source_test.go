package runtime_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWorkspaceAdvance(t *testing.T) {
	Convey("Source nodes run inside the first group and retain each observation", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		first := &recordingStage{source: func(observation recordedObservation) string {
			return fmt.Sprintf("ticker-%d", observation.sequence)
		}}
		second := &recordingStage{source: func(observation recordedObservation) string {
			return fmt.Sprintf("trade-%d", observation.sequence)
		}}
		downstream := &recordingStage{echo: true, before: func(observation recordedObservation) error {
			cycle := observation.sequence / 2
			if len(first.snapshot()) <= int(cycle) || len(second.snapshot()) <= int(cycle) {
				return fmt.Errorf("source group was not complete at %d", cycle)
			}
			return nil
		}}
		fixture, err := newWorkspaceFixture(ctx, 2, []*recordingStage{first, second}, []*recordingStage{downstream})
		So(err, ShouldBeNil)
		defer fixture.release()
		for range 31 {
			So(fixture.publish(ctx), ShouldBeNil)
		}
		So(fixture.flush(ctx), ShouldBeNil)
		expected := make([]recordedObservation, 0, 64)
		for sequence := range 64 {
			kind := "ticker"
			if sequence%2 == 1 {
				kind = "trade"
			}
			expected = append(expected, recordedObservation{77, int64(sequence), fmt.Sprintf("%s-%d", kind, sequence)})
		}
		So(downstream.snapshot(), ShouldResemble, expected)
		So(fixture.publish(ctx, "outside-the-group"), ShouldNotBeNil)
	})
	Convey("A blocked source holds the next group even after a peer returns", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		gate := make(chan struct{})
		entered := make(chan struct{}, 1)
		first := &recordingStage{source: func(recordedObservation) string { return "ticker" }}
		second := &recordingStage{entered: entered, gate: gate, source: func(recordedObservation) string { return "trade" }}
		downstream := &recordingStage{}
		completed := make(chan error, 1)
		go func() {
			fixture, err := newWorkspaceFixture(ctx, 2, []*recordingStage{first, second}, []*recordingStage{downstream})
			if err == nil {
				fixture.release()
			}
			completed <- err
		}()
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		So(downstream.snapshot(), ShouldBeEmpty)
		close(gate)
		So(<-completed, ShouldBeNil)
		So(downstream.snapshot(), ShouldResemble, []recordedObservation{{77, 0, "ticker"}, {77, 1, "trade"}})
	})
}

func BenchmarkWorkspaceAdvance(b *testing.B) {
	ctx := context.Background()
	source := func(recordedObservation) string {
		return `{"channel":"ticker","data":{"symbol":"BTC/USD","last":65000}}`
	}
	fixture, err := newWorkspaceFixture(ctx, 64, []*recordingStage{{source: source}, {source: source}, {source: source}}, []*recordingStage{{}, {}})
	if err != nil {
		b.Fatal(err)
	}
	defer fixture.release()
	b.ReportAllocs()
	for b.Loop() {
		if err := fixture.publish(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
