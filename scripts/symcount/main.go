package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/fasthttp/websocket"
)

// symcount tallies the DISTINCT symbols appearing on the hub websocket, so we
// know empirically whether the system runs one pair or hundreds.
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

	symbols := map[string]int{}
	gaugeSymbols := map[string]map[string]struct{}{} // source -> set of symbols
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	_ = conn.SetReadDeadline(deadline)

	for time.Now().Before(deadline) {
		var f map[string]any
		if err := conn.ReadJSON(&f); err != nil {
			break
		}
		sym, _ := f["symbol"].(string)
		if sym != "" {
			symbols[sym]++
		}
		if f["chart"] == "gauge" {
			src, _ := f["source"].(string)
			if gaugeSymbols[src] == nil {
				gaugeSymbols[src] = map[string]struct{}{}
			}
			if sym != "" {
				gaugeSymbols[src][sym] = struct{}{}
			}
		}
	}

	syms := make([]string, 0, len(symbols))
	for s := range symbols {
		syms = append(syms, s)
	}
	sort.Slice(syms, func(i, j int) bool { return symbols[syms[i]] > symbols[syms[j]] })
	fmt.Printf("=== DISTINCT symbols on the wire: %d ===\n", len(syms))
	for i, s := range syms {
		if i >= 25 {
			fmt.Printf("  ... and %d more\n", len(syms)-25)
			break
		}
		fmt.Printf("  %-14s %d frames\n", s, symbols[s])
	}

	fmt.Println("=== distinct symbols per gauge source ===")
	srcs := make([]string, 0, len(gaugeSymbols))
	for s := range gaugeSymbols {
		srcs = append(srcs, s)
	}
	sort.Strings(srcs)
	for _, s := range srcs {
		fmt.Printf("  %-12s %d symbols\n", s, len(gaugeSymbols[s]))
	}
}
