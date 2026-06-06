package io

import (
	"os"

	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

/*
WriteThoughts serializes a reasoning forest to a playbook file — the optimizer's
output, read back live by the story. It is the inverse of reasoning.ParseThoughts.
*/
func WriteThoughts(path string, thoughts []reasoning.Thought) error {
	encoded, err := reasoning.MarshalThoughts(thoughts, 2)
	if err != nil {
		return err
	}

	return os.WriteFile(path, encoded, 0o644)
}
