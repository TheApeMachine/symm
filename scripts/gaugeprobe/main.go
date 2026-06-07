package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/fasthttp/websocket"
)

// gaugeprobe captures chart=gauge frames and reports, per source, how many
// frames arrived and the latest warmup-relevant fields (calibrated, samples,
// min_samples, confidence). A source that never appears emits no gauge frame.
func main() {
	seconds := 12
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &seconds)
	}

	conn, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8765/ws", nil)
	if err != nil {
		fmt.Println("DIAL ERROR:", err)
		os.Exit(1)
	}
	defer conn.Close()

	type stat struct {
		count                          int
		calibrated                     any
		samples, minSamples, confidence any
	}
	gauges := map[string]*stat{}
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	_ = conn.SetReadDeadline(deadline)

	for time.Now().Before(deadline) {
		var f map[string]any
		if err := conn.ReadJSON(&f); err != nil {
			break
		}
		if f["chart"] != "gauge" {
			continue
		}
		src, _ := f["source"].(string)
		s := gauges[src]
		if s == nil {
			s = &stat{}
			gauges[src] = s
		}
		s.count++
		s.calibrated = f["calibrated"]
		s.samples = f["samples"]
		s.minSamples = f["min_samples"]
		s.confidence = f["confidence"]
	}

	keys := make([]string, 0, len(gauges))
	for k := range gauges {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("=== gauge sources seen (%ds) ===\n", seconds)
	for _, k := range keys {
		s := gauges[k]
		fmt.Printf("  %-12s count=%-4d calibrated=%-6v samples=%-6v min_samples=%-6v confidence=%v\n",
			k, s.count, s.calibrated, s.samples, s.minSamples, s.confidence)
	}

	// Report which configured sources never emitted a gauge frame.
	all := []string{"hawkes", "fluid", "pumpdump", "causal", "depthflow", "leadlag",
		"liquidity", "sentiment", "toxicity", "correlation", "exhaustion", "prediction", "cvd"}
	missing := []string{}
	for _, src := range all {
		if gauges[src] == nil {
			missing = append(missing, src)
		}
	}
	fmt.Println("=== configured sources with NO gauge frame ===")
	fmt.Println(" ", missing)
}
