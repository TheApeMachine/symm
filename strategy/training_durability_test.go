package strategy

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestStageSurfacesPersistError(t *testing.T) {
	Convey("Given a checkpointed model with a failed Iceberg persist", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()

		training.mu.Lock()
		training.checkpointed = true
		training.blocked = true
		training.persistErr = errors.New("iceberg unavailable")
		training.mu.Unlock()

		_, stage, detail := training.stage()
		So(stage, ShouldEqual, "HISTORICAL VALIDATION")
		So(detail, ShouldContainSubstring, "durability blocked")
		So(detail, ShouldContainSubstring, "iceberg unavailable")
	})
}

func TestFinishWriteRequeuesWithoutClearingBlocked(t *testing.T) {
	Convey("Given CommitReady failure, the batch is requeued and stays blocked", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := NewTraining(ctx, 1, priced(t, "BTC/USD", 0.1), nil, nil, nil)
		batch := []heldEpisode{{
			record: tables.ExcursionRecord{
				ID:         "BTC/USD:1:2",
				Symbol:     "BTC/USD",
				AnchorTick: 1,
				ExitTick:   2,
			},
		}}

		training.mu.Lock()
		training.writing = true
		training.blocked = true
		training.mu.Unlock()

		training.finishWrite(batch, errors.New("append failed"))

		training.mu.Lock()
		defer training.mu.Unlock()
		So(training.writing, ShouldBeFalse)
		So(training.blocked, ShouldBeTrue)
		So(len(training.queue), ShouldEqual, 1)
		So(training.persistErr, ShouldNotBeNil)
	})
}

func TestPersistLoopDrainsWithCatalog(t *testing.T) {
	Convey("Given a real catalog, enqueue clears durability blocked after CommitReady", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		training := NewTraining(ctx, 7, priced(t, "BTC/USD", 0.1), nil, catalog, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.mu.Unlock()

		err := training.enqueue(heldEpisode{
			record: tables.ExcursionRecord{
				ID:                 "BTC/USD:drain:1",
				Symbol:             "BTC/USD",
				Direction:          "up",
				PrecursorStartTick: 1,
				AnchorTick:         2,
				ExitTick:           3,
				Epoch:              7,
			},
		})
		So(err, ShouldBeNil)

		deadline := time.Now().Add(5 * time.Second)
		for {
			training.mu.Lock()
			blocked := training.blocked
			writing := training.writing
			qlen := len(training.queue)
			perr := training.persistErr
			training.mu.Unlock()

			if !blocked && !writing && qlen == 0 && perr == nil {
				break
			}

			if time.Now().After(deadline) {
				t.Fatalf("still blocked writing=%v queue=%d err=%v", writing, qlen, perr)
			}

			time.Sleep(20 * time.Millisecond)
		}

		_, _, detail := training.stage()
		So(detail, ShouldNotContainSubstring, "durability blocked")

		rows, err := catalog.Excursions(ctx, 7, nil)
		So(err, ShouldBeNil)
		So(len(rows), ShouldBeGreaterThan, 0)
	})
}

func TestWaitDrainedDoesNotBailOnTransientPersistErr(t *testing.T) {
	Convey("Given a requeued failed batch that later succeeds, waitDrained returns nil", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		training := NewTraining(ctx, 9, priced(t, "BTC/USD", 0.1), nil, catalog, nil)

		batch := []heldEpisode{{
			record: tables.ExcursionRecord{
				ID:         "BTC/USD:wait:1",
				Symbol:     "BTC/USD",
				AnchorTick: 1,
				ExitTick:   2,
				Epoch:      9,
			},
		}}

		// Simulate a failed write that requeues (the old waitDrained bailed here).
		training.finishWrite(batch, errors.New("transient"))

		training.mu.Lock()
		So(training.persistErr, ShouldNotBeNil)
		So(len(training.queue), ShouldEqual, 1)
		training.mu.Unlock()

		// Wake persistLoop to retry against a healthy catalog.
		select {
		case training.wake <- struct{}{}:
		default:
		}

		done := make(chan error, 1)
		go func() { done <- training.waitDrained() }()

		select {
		case err := <-done:
			So(err, ShouldBeNil)
		case <-time.After(5 * time.Second):
			t.Fatal("waitDrained timed out")
		}
	})
}

func TestCommitExcursionsTimesOutWhenGateHeld(t *testing.T) {
	Convey("Given a held commit gate, CommitExcursions returns ctx error and finishWrite clears writing", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		held := make(chan struct{})
		release := make(chan struct{})

		go func() {
			_ = catalog.WithCommit(context.Background(), func() error {
				close(held)
				<-release
				return nil
			})
		}()
		<-held

		training := NewTraining(ctx, 11, priced(t, "BTC/USD", 0.1), nil, catalog, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.writing = true
		training.blocked = true
		training.mu.Unlock()

		writer := tables.NewWriter(catalog, 11)
		writer.AddExcursion(tables.ExcursionRecord{
			ID: "BTC/USD:timeout:1", Symbol: "BTC/USD", AnchorTick: 1, ExitTick: 2, Epoch: 11,
		})
		commitCtx, commitCancel := context.WithTimeout(ctx, 80*time.Millisecond)
		err := writer.CommitExcursions(commitCtx)
		commitCancel()
		So(err, ShouldNotBeNil)

		training.finishWrite([]heldEpisode{{
			record: tables.ExcursionRecord{ID: "BTC/USD:timeout:1", Symbol: "BTC/USD"},
		}}, err)

		training.mu.Lock()
		writing := training.writing
		blocked := training.blocked
		perr := training.persistErr
		training.mu.Unlock()
		So(writing, ShouldBeFalse)
		So(blocked, ShouldBeTrue)
		So(perr, ShouldNotBeNil)

		_, _, detail := training.stage()
		So(detail, ShouldContainSubstring, "durability blocked")
		So(detail, ShouldNotEqual, "durability blocked: writing")

		close(release)
	})
}


func TestCommitExcursionsBoundedSurfacesTimeout(t *testing.T) {
	Convey("Given a held commit gate, bounded commit sets persistErr before Append returns", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		held := make(chan struct{})
		release := make(chan struct{})

		go func() {
			_ = catalog.WithCommit(context.Background(), func() error {
				close(held)
				<-release
				return nil
			})
		}()
		<-held

		training := NewTraining(ctx, 13, priced(t, "BTC/USD", 0.1), nil, catalog, nil)
		frame := regionFrame("BTC/USD", 1, 2)
		training.grid.Update(frame)
		training.grid.Settle()
		training.mu.Lock()
		training.checkpointed = true
		training.writing = true
		training.blocked = true
		training.mu.Unlock()

		writer := tables.NewWriter(catalog, 13)
		writer.AddExcursion(tables.ExcursionRecord{
			ID: "BTC/USD:bounded:1", Symbol: "BTC/USD", AnchorTick: 1, ExitTick: 2, Epoch: 13,
		})

		commitCtx, commitCancel := context.WithTimeout(ctx, 80*time.Millisecond)
		errCh := make(chan error, 1)
		go func() {
			errCh <- training.commitExcursionsBounded(commitCtx, writer)
		}()

		deadline := time.Now().Add(2 * time.Second)
		for {
			training.mu.Lock()
			perr := training.persistErr
			training.mu.Unlock()
			if perr != nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("persistErr not set while Append blocked on gate")
			}
			time.Sleep(10 * time.Millisecond)
		}

		_, _, detail := training.stage()
		So(detail, ShouldContainSubstring, "durability blocked")
		So(detail, ShouldNotEqual, "durability blocked: writing")

		close(release)
		commitCancel()
		<-errCh
	})
}
