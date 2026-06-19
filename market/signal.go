package market

import "github.com/theapemachine/datura"

/*
Signal is a mechanism to structure raw market data into
measurements, which are labeled as semantic categories.
*/
type Signal interface {
	Measure(*datura.Artifact) *datura.Artifact
	Close() error
}
