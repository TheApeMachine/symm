package io

import (
	"os"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
WriteThoughts serializes a reasoning forest to a playbook file — the optimizer's
output, read back live by the story. It is the inverse of perspectives.ParseThoughts.
*/
func WriteThoughts(path string, thoughts []perspectives.Thought) error {
	encoded, err := perspectives.MarshalThoughts(thoughts, 2)
	if err != nil {
		return err
	}

	return os.WriteFile(path, encoded, 0o644)
}
