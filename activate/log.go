package activate

import (
	"fmt"
	"sync"
)

var seen sync.Map

/*
Once prints a single activation line for key. Safe to call from hot paths.
*/
func Once(key string) {
	if _, loaded := seen.LoadOrStore(key, struct{}{}); loaded {
		return
	}

	fmt.Println("[symm] activate:", key)
}

/*
Boot prints a startup milestone (not deduplicated).
*/
func Boot(message string) {
	fmt.Println("[symm] boot:", message)
}
