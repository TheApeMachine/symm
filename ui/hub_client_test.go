package ui

import "testing"

func TestWsClientSubscribeFiltersSymbols(t *testing.T) {
	client := &wsClient{}

	if client.wantsSymbol("BTC/EUR") {
		t.Fatal("expected no symbols before subscribe")
	}

	client.subscribe([]string{"BTC/EUR", "ETH/EUR"})

	if !client.wantsSymbol("BTC/EUR") {
		t.Fatal("expected BTC/EUR after subscribe")
	}

	if client.wantsSymbol("ZETA/EUR") {
		t.Fatal("expected ZETA/EUR to remain unsubscribed")
	}

	client.unsubscribe([]string{"ETH/EUR"})

	if client.wantsSymbol("ETH/EUR") {
		t.Fatal("expected ETH/EUR removed after unsubscribe")
	}
}

func TestHandleClientMessageSubscribe(t *testing.T) {
	client := &wsClient{}
	hub := &Hub{}

	hub.handleClientMessage(client, []byte(`{"op":"subscribe","symbols":["DEEP/EUR"]}`))

	if !client.wantsSymbol("DEEP/EUR") {
		t.Fatal("expected subscribe message to register symbol")
	}
}
