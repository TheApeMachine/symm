package core

import (
	"strings"
	"testing"
)

func TestKrakenEndpointsAreAbsoluteHTTPSOrWSS(t *testing.T) {
	if !strings.HasPrefix(KRAKEN_API_URL, "https://") {
		t.Fatalf("expected https api url, got %q", KRAKEN_API_URL)
	}

	if !strings.HasPrefix(KRAKEN_BASE_URL, "https://") {
		t.Fatalf("expected https base url, got %q", KRAKEN_BASE_URL)
	}

	if !strings.HasPrefix(KRAKEN_WS_URL, "wss://") {
		t.Fatalf("expected wss websocket url, got %q", KRAKEN_WS_URL)
	}
}

func TestChannelNamesAreNonEmpty(t *testing.T) {
	channels := []string{
		ChannelInstrument,
		ChannelTicker,
		ChannelBook,
		ChannelTrades,
		ChannelExecutions,
		ChannelBalances,
	}

	for _, channel := range channels {
		if strings.TrimSpace(channel) == "" {
			t.Fatalf("expected non-empty channel name")
		}
	}
}
