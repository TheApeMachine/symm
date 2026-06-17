package cognitive

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
)

const sensoryNamespace = "s/"

/*
SharedTree returns the process-wide DMT tree singleton.
Every caller shares the same radix root via dmt.NewTree sync.Once.
*/
func SharedTree() *dmt.Tree {
	persistDir := viper.GetString("cognitive.persist_dir")

	return dmt.NewTree(persistDir)
}

func sensorySearchPrefix(regimePrefix []byte) []byte {
	searchPrefix := make([]byte, len(sensoryNamespace)+len(regimePrefix))
	copy(searchPrefix, sensoryNamespace)
	copy(searchPrefix[len(sensoryNamespace):], regimePrefix)

	return searchPrefix
}

func execProfileKey(sequence []byte) []byte {
	profileKey := make([]byte, len("exec/")+len(sequence))
	copy(profileKey, "exec/")
	copy(profileKey[len("exec/"):], sequence)

	return profileKey
}
