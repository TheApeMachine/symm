package cmd

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"
)

/*
startProfileServer listens when SYMM_PPROF is set. Use "1" for 127.0.0.1:6060 or
pass a host:port. Capture CPU with:

	curl -o runs/profiles/replay-cpu.prof 'http://127.0.0.1:6060/debug/pprof/profile?seconds=30'
*/
func startProfileServer() {
	raw := strings.TrimSpace(os.Getenv("SYMM_PPROF"))

	if raw == "" {
		return
	}

	addr := raw

	if raw == "1" || strings.EqualFold(raw, "true") {
		addr = "127.0.0.1:6060"
	}

	go func() {
		log.Printf("symm: pprof listening on http://%s/debug/pprof/", addr)

		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Printf("symm: pprof server stopped: %v", err)
		}
	}()

	fmt.Fprintf(os.Stderr, "symm: pprof http://%s/debug/pprof/\n", addr)
}
