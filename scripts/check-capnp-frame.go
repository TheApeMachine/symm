//go:build ignore

// Standalone dependency check: use the actual Capnp RPC schema, not a replica.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"capnproto.org/go/capnp/v3"
	protocol "capnproto.org/go/capnp/v3/std/capnp/rpc"
)

type result struct {
	File string `json:"file"`
	Expected uint32 `json:"expected"`
	Actual uint32 `json:"actual"`
	Passed bool `json:"passed"`
	Failure string `json:"failure,omitempty"`
}

func readQuestion(filename string) (uint32, error) {
	encoded, err := os.ReadFile(filename)
	if err != nil { return 0, err }
	message, err := capnp.Unmarshal(encoded)
	if err != nil { return 0, err }
	defer message.Release()
	root, err := protocol.ReadRootMessage(message)
	if err != nil { return 0, err }
	if root.Which() != protocol.Message_Which_bootstrap {
		return 0, fmt.Errorf("expected bootstrap, got %v", root.Which())
	}
	bootstrap, err := root.Bootstrap()
	if err != nil { return 0, err }
	return bootstrap.QuestionId(), nil
}

func writeFixture(directory string) error {
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil { return err }
	defer message.Release()
	root, err := protocol.NewRootMessage(segment)
	if err != nil { return err }
	bootstrap, err := root.NewBootstrap()
	if err != nil { return err }
	bootstrap.SetQuestionId(41)
	encoded, err := message.Marshal()
	if err != nil { return err }
	return os.WriteFile(filepath.Join(directory, "go-bootstrap.capnp"), encoded, 0644)
}

func run() error {
	if len(os.Args) == 3 && os.Args[1] == "--write" {
		return writeFixture(os.Args[2])
	}
	if len(os.Args) != 2 { return fmt.Errorf("usage: check-capnp-frame [--write] DIRECTORY") }
	directory := os.Args[1]
	results := []result{
		{File: "go-bootstrap.capnp", Expected: 41},
		{File: "single.capnp", Expected: 41},
		{File: "multi.capnp", Expected: 43},
	}
	failed := false
	for index := range results {
		entry := &results[index]
		actual, err := readQuestion(filepath.Join(directory, entry.File))
		entry.Actual = actual
		entry.Passed = err == nil && actual == entry.Expected
		if err != nil { entry.Failure = err.Error() }
		if err == nil && !entry.Passed { entry.Failure = "question ID changed across implementations" }
		if !entry.Passed { failed = true }
	}
	encoded, err := json.MarshalIndent(results, "", "  ")
	if err != nil { return err }
	if err := os.WriteFile(filepath.Join(directory, "go-results.json"), append(encoded, '\n'), 0644); err != nil { return err }
	fmt.Println(string(encoded))
	if failed { return fmt.Errorf("Capnp interoperability assertions failed; see go-results.json") }
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
