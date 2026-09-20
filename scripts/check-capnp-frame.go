//go:build ignore

// This is run in an isolated test module by capnp-repair.yml. It deliberately
// uses the project's Capnp version, not a replacement serialization library.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"capnproto.org/go/capnp/v3"
	protocol "capnproto.org/go/capnp/v3/std/capnp/rpc"
)

func verify(filename string, expected uint32) error {
	encoded, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	message, err := capnp.Unmarshal(encoded)
	if err != nil {
		return err
	}
	defer message.Release()
	root, err := protocol.ReadRootMessage(message)
	if err != nil {
		return err
	}
	bootstrap, err := root.Bootstrap()
	if err != nil {
		return err
	}
	if bootstrap.QuestionId() != expected {
		return fmt.Errorf("%s: question ID %d, expected %d", filename, bootstrap.QuestionId(), expected)
	}
	fmt.Printf("PASS %s: Go Capnp decoded question ID %d across %d segments\n", filepath.Base(filename), expected, message.NumSegments())
	return nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: check-capnp-frame DIRECTORY")
		os.Exit(2)
	}
	for filename, expected := range map[string]uint32{"single.capnp": 41, "multi.capnp": 43} {
		if err := verify(filepath.Join(os.Args[1], filename), expected); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
