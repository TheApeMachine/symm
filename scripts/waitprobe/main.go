package main

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/qpool"
)

// waitprobe isolates the qpool BroadcastConsumer.Wait blocking behaviour.
// Hypothesis: Wait() parks via fast_park (-> _Gwaiting) with no corresponding
// goready when ring.Push happens, so the first empty Pop parks the goroutine
// forever. Prediction: case B (Send AFTER Wait starts) hangs even past the
// 2s context deadline, because the parked goroutine can never re-check ctx.
func main() {
	// Case A: value is already in the ring before Wait -> first Pop succeeds.
	caseA()
	// Case B: Wait starts on an empty ring, value sent 300ms later.
	caseB()
}


func caseA() {
	fmt.Println("CASE A: value present before Wait (expect: returns immediately)")
	pool := qpool.NewQ[any](context.Background(), 1, 4, nil)
	bg := pool.CreateBroadcastGroup("A", time.Second)
	consumer := bg.Subscribe("consumer", 16)
	bg.Send(&qpool.QValue[any]{Value: "hello"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	runWait(consumer, ctx)
}

func caseB() {
	fmt.Println("CASE B: empty ring, value sent 300ms after Wait starts (expect: should return ~300ms)")
	pool := qpool.NewQ[any](context.Background(), 1, 4, nil)
	bg := pool.CreateBroadcastGroup("B", time.Second)
	consumer := bg.Subscribe("consumer", 16)

	go func() {
		t := time.NewTimer(300 * time.Millisecond)
		<-t.C
		bg.Send(&qpool.QValue[any]{Value: "hello"})
		fmt.Println("  [producer] Send() done")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	runWait(consumer, ctx)
}

func runWait(consumer *qpool.BroadcastConsumer, ctx context.Context) {
	done := make(chan string, 1)
	start := time.Now()

	go func() {
		v, err := consumer.Wait(ctx)
		if err != nil {
			done <- fmt.Sprintf("Wait returned err=%v after %v", err, time.Since(start))
			return
		}
		done <- fmt.Sprintf("Wait returned value=%v after %v", v.Value, time.Since(start))
	}()

	select {
	case msg := <-done:
		fmt.Println("  RESULT:", msg)
	case <-time.After(4 * time.Second):
		fmt.Println("  RESULT: *** HANG *** Wait never returned after 4s (parked forever, ctx deadline ignored)")
	}
	fmt.Println()
}
