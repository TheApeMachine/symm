package numeric

import (
	"hash/fnv"

	"github.com/theapemachine/errnie"
)

/*
HashString hashes a string to a 64-bit unsigned integer
using FNV-1a. This is a good hash function for strings.
*/
func HashString(s string) (uint64, error) {
	hasher := fnv.New64a()

	if _, err := hasher.Write([]byte(s)); err != nil {
		return 0, errnie.Error(err)
	}

	return hasher.Sum64(), nil
}
