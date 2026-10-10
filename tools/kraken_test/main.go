package main

import (
	"context"
	"fmt"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
)

func main() {
	client := network.NewWebsocketClient(context.Background())
	msg, _ := sonic.Marshal(kraken.NewInstrumentSubscription())
	client.Write(msg)

	for range 5 {
		buf, err := client.Read()
		if err != nil {
			fmt.Println("Error:", err)
			break
		}
		fmt.Printf("Received: %s\n", buf)
	}
}
