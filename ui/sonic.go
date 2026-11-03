package ui

import "github.com/bytedance/sonic"

// Sonic is frozen once at startup so dashboard frames reuse JIT encoders.
// NaN / ±Inf become JSON null instead of aborting the publish.
var Sonic = sonic.Config{
	EncodeNullForInfOrNan: true,
}.Froze()
