package websocket

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestSimulatorRecordConcurrent(t *testing.T) {
	simulator := NewSimulator()

	if err := simulator.Initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	for idx := 0; idx < 64; idx++ {
		simulator.Record(WEBSOCKET, 0)
		simulator.Record(REST, 0)
	}

	var wg sync.WaitGroup

	record := func(latencyType LatencyType) {
		defer wg.Done()

		for idx := 0; idx < 10_000; idx++ {
			simulator.Record(latencyType, time.Duration(idx%3))

			if idx%97 == 0 {
				runtime.Gosched()
			}
		}
	}

	do := func(latencyType LatencyType) {
		defer wg.Done()

		for idx := 0; idx < 10_000; idx++ {
			simulator.Do(latencyType, func() {})

			if idx%97 == 0 {
				runtime.Gosched()
			}
		}
	}

	wg.Add(3)
	go record(WEBSOCKET)
	go record(REST)
	go do(WEBSOCKET)
	wg.Wait()
}
