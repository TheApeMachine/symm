package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/fasthttp/websocket"
)

// wsprobe connects to the symm UI hub websocket and tallies the frames it
// receives by their routing key (event / chart / source) so we can see exactly
// what the backend fans out. Run: go run ./scripts/wsprobe [seconds]
func main() {
	seconds := 15
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &seconds)
	}

	url := "ws://127.0.0.1:8765/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		fmt.Println("DIAL ERROR:", err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Println("connected to", url)

	counts := map[string]int{}
	total := 0
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	_ = conn.SetReadDeadline(deadline)

	for time.Now().Before(deadline) {
		var frame map[string]any
		if err := conn.ReadJSON(&frame); err != nil {
			fmt.Println("read stopped:", err)
			break
		}
		total++
		key := ""
		if ev, ok := frame["event"].(string); ok {
			key = "event=" + ev
		} else if ch, ok := frame["chart"].(string); ok {
			key = "chart=" + ch
			if src, ok := frame["source"].(string); ok {
				key += ",source=" + src
			}
		} else if sym, ok := frame["symbol"].(string); ok {
			key = "symbol-frame=" + sym
		} else {
			keys := make([]string, 0, len(frame))
			for k := range frame {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			key = fmt.Sprintf("other:%v", keys)
		}
		counts[key]++
		if total <= 12 {
			fmt.Printf("  frame %d: %s\n", total, key)
		}
	}

	fmt.Println("=== TOTAL frames:", total, "===")
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %4d  %s\n", counts[k], k)
	}
}
