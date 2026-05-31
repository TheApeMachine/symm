//go:build race

package market

func raceDetectorActive() bool {
	return true
}
