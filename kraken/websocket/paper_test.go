package websocket

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPaperExecute(t *testing.T) {
	Convey("Given concurrent commands targeting one native paper ledger", t, func() {
		temporaryDirectory := t.TempDir()
		guardDirectory := filepath.Join(temporaryDirectory, "active")
		binaryPath := filepath.Join(temporaryDirectory, "kraken")
		script := []byte(`#!/bin/sh
if ! mkdir "$KRAKEN_TEST_GUARD" 2>/dev/null; then
    echo "overlapping paper command" >&2
    exit 1
fi
trap 'rmdir "$KRAKEN_TEST_GUARD"' EXIT
sleep 0.05
printf '{}'
`)
		So(os.WriteFile(binaryPath, script, 0o755), ShouldBeNil)
		t.Setenv("KRAKEN_TEST_GUARD", guardDirectory)
		t.Setenv("PATH", temporaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
		paper := &Paper{ctx: t.Context()}
		const parallelCommands = 4
		start := make(chan struct{})
		results := make(chan error, parallelCommands)
		var waitGroup sync.WaitGroup

		for range parallelCommands {
			waitGroup.Add(1)

			go func() {
				defer waitGroup.Done()
				<-start
				_, err := paper.execute("status", "status")
				results <- err
			}()
		}

		Convey("It should serialize every CLI process that can read or write the state file", func() {
			close(start)
			waitGroup.Wait()
			close(results)

			for err := range results {
				So(err, ShouldBeNil)
			}
		})
	})
}
