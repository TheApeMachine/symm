package hindsight

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCanceledLoadDoesNotFailTheWaiter(t *testing.T) {
	service := &Service{
		tapes:    map[uint64]*runTape{},
		tapeWait: map[uint64]chan struct{}{},
		tapeErr:  map[uint64]error{},
	}

	started := make(chan struct{})
	release := make(chan struct{})
	var loads sync.WaitGroup
	loads.Add(2)

	load := func(ctx context.Context) (*runTape, error) {
		defer loads.Done()

		select {
		case <-started:
		default:
			close(started)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return &runTape{bySymbol: map[string][]stitch{}}, nil
		}
	}

	first, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.loadTape(first, 7, load)
		firstDone <- err
	}()

	<-started

	secondDone := make(chan error, 1)
	go func() {
		_, err := service.loadTape(context.Background(), 7, load)
		secondDone <- err
	}()

	// The second caller is blocked on the first load. Canceling the first
	// must not become the second caller's result.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-firstDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("first load err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled load did not return")
	}

	close(release)

	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("waiter inherited the canceled load: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter did not load the run itself")
	}

	loads.Wait()
}
