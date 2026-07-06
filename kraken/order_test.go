package kraken

import "testing"

func BenchmarkNewPaperOrderSlice(benchmarkTB *testing.B) {
	buf := []byte(`[{"id":"PAPER-00003","pair":"BTCUSD","price":90000,"reserved_amount":9,"reserved_asset":"USD","side":"buy","type":"limit","volume":0.0001,"created_at":"2026-07-06T10:00:00Z"}]`)

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		_ = NewPaperOrderSlice(buf)
	}
}
