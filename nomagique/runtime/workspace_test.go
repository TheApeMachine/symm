package runtime

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRingFIFO(t *testing.T) {
	ring := NewRing[int](4)

	for value := 1; value <= 4; value++ {
		ring.Push(value)
	}

	for want := 1; want <= 4; want++ {
		got, ok := ring.Pop()

		if !ok || got != want {
			t.Fatalf("pop %d: got %d ok=%v", want, got, ok)
		}
	}

	if _, ok := ring.Pop(); ok {
		t.Fatal("empty ring returned a value")
	}
}

func TestRingOverwriteOldest(t *testing.T) {
	ring := NewRing[int](4)

	for value := 1; value <= 6; value++ {
		ring.Push(value)
	}

	if dropped := ring.Dropped(); dropped != 2 {
		t.Fatalf("dropped=%d want=2", dropped)
	}

	for want := 3; want <= 6; want++ {
		got, ok := ring.Pop()

		if !ok || got != want {
			t.Fatalf("pop %d: got %d ok=%v", want, got, ok)
		}
	}

	if length := ring.Length(); length != 0 {
		t.Fatalf("length=%d want=0 after full drain", length)
	}
}

func TestRingConcurrentProducers(t *testing.T) {
	const producers = 4
	const perProducer = 1000

	ring := NewRing[*int](4096)
	var group sync.WaitGroup

	for producerIndex := range producers {
		group.Add(1)

		go func() {
			defer group.Done()

			for valueIndex := range perProducer {
				value := producerIndex*perProducer + valueIndex
				ring.Push(&value)
			}
		}()
	}

	group.Wait()

	seen := make(map[int]bool)
	order := make([]int, 0, producers*perProducer)

	for {
		value, ok := ring.Pop()

		if !ok {
			break
		}

		if seen[*value] {
			t.Fatalf("value %d consumed twice", *value)
		}

		seen[*value] = true
		order = append(order, *value)
	}

	if len(order) != producers*perProducer {
		t.Fatalf("consumed %d values, want %d", len(order), producers*perProducer)
	}

	// Each producer's values must appear in push order.
	lastSeen := make([]int, producers)

	for _, value := range order {
		producerIndex := value / perProducer
		valueIndex := value % perProducer

		if valueIndex < lastSeen[producerIndex] {
			t.Fatalf("producer %d out of order: %v", producerIndex, order)
		}

		lastSeen[producerIndex] = valueIndex
	}
}

type busItem struct {
	key   string
	value int
}

func TestWorkspaceChannelFansOutToEverySubscription(t *testing.T) {
	workspace := NewWorkspace(nil)
	defer workspace.Close()
	channel := ChannelOf(workspace, "items", func(item busItem) string {
		return item.key
	})

	var mu sync.Mutex
	seenA := map[string][]int{}
	seenB := map[string][]int{}

	channel.Subscribe("a", func(item busItem) error {
		mu.Lock()
		seenA[item.key] = append(seenA[item.key], item.value)
		mu.Unlock()
		return nil
	})
	channel.Subscribe("b", func(item busItem) error {
		mu.Lock()
		seenB[item.key] = append(seenB[item.key], item.value)
		mu.Unlock()
		return nil
	})

	for value := 0; value < 8; value++ {
		channel.Publish(busItem{key: "BTC", value: value})
		channel.Publish(busItem{key: "ETH", value: value})
	}

	deadline := time.Now().Add(3 * time.Second)

	for channel.Snapshot().Completed < 32 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if got := channel.Snapshot().Completed; got != 32 {
		t.Fatalf("completed=%d want=32", got)
	}

	for _, seen := range []map[string][]int{seenA, seenB} {
		for _, key := range []string{"BTC", "ETH"} {
			mu.Lock()
			values := append([]int(nil), seen[key]...)
			mu.Unlock()

			for index, value := range values {
				if value != index {
					t.Fatalf("%s order=%v", key, values)
				}
			}
		}
	}
}

func TestWorkspaceRunsKeysConcurrently(t *testing.T) {
	workspace := NewWorkspace(nil)
	defer workspace.Close()
	channel := ChannelOf(workspace, "concurrent", func(item busItem) string {
		return item.key
	})

	active := atomic.Int32{}
	peak := atomic.Int32{}

	channel.Subscribe("consumer", func(item busItem) error {
		current := active.Add(1)
		for {
			old := peak.Load()
			if current <= old || peak.CompareAndSwap(old, current) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		active.Add(-1)
		return nil
	})

	for value := 0; value < 8; value++ {
		for _, key := range []string{"BTC", "ETH", "SOL"} {
			channel.Publish(busItem{key: key, value: value})
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for channel.Snapshot().Completed < 24 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := channel.Snapshot().Completed; got != 24 {
		t.Fatalf("completed=%d want=24", got)
	}
	if peak.Load() < 2 {
		t.Fatalf("keys did not execute concurrently; peak=%d", peak.Load())
	}
}

func TestWorkspaceRetainsFirstError(t *testing.T) {
	workspace := NewWorkspace(nil)
	defer workspace.Close()
	channel := ChannelOf(workspace, "errors", func(value int) string { return "one" })

	channel.Subscribe("consumer", func(value int) error {
		return fmt.Errorf("failed %d", value)
	})
	channel.Publish(7)

	deadline := time.Now().Add(time.Second)
	for channel.Error() == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if channel.Error() == nil {
		t.Fatal("channel did not retain step failure")
	}
}

func TestWorkspaceDropsWhenSubscriptionSaturated(t *testing.T) {
	workspace := NewWorkspace(nil)
	defer workspace.Close()
	channel := ChannelOf(workspace, "saturated", func(value int) string { return "one" })

	block := make(chan struct{})
	released := make(chan struct{})
	var started atomic.Bool

	channel.Subscribe("consumer", func(value int) error {
		if started.CompareAndSwap(false, true) {
			close(block)
			<-released
		}
		return nil
	})

	channel.Publish(1)
	<-block

	for index := 0; index < 20000; index++ {
		channel.Publish(2)
	}

	close(released)

	if channel.Snapshot().Dropped == 0 {
		t.Fatal("no drops counted while the subscription was saturated")
	}
}
