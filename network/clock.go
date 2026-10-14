package network

import (
	_ "unsafe"
)

//go:linkname SetSynthetic time.SetSynthetic
func SetSynthetic(unixNano int64)
