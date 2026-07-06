package websocket

import "testing"

func BenchmarkStreamReceive(benchmarkTB *testing.B) {
	stream := NewStream(64)
	ticker := stream.Observe("ticker")
	frame := []byte(`{
		"channel": "ticker",
		"type": "update",
		"data": [{"symbol": "BTC/USD", "last": 100.0}]
	}`)

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		stream.Receive(frame)
		<-ticker
	}
}
