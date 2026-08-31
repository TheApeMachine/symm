/*
hindsight_probe serves the Hindsight inspection reads over an existing capture
database, with no live market connection and no trading path. It exists so the
inspection surface can be exercised against a real recorded run without
starting the trading process.
*/
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/ui"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: hindsight_probe <events.sqlite> [listen address]")
		os.Exit(1)
	}

	address := "127.0.0.1:8765"

	if len(os.Args) > 2 {
		address = os.Args[2]
	}

	viper.Set("ui.addr", address)

	engine, err := store.NewSQLite(os.Args[1])

	if err != nil {
		panic(err)
	}

	defer engine.Close()

	hub := ui.NewHub(context.Background())
	hub.SetHindsightStore(engine)

	fmt.Println("hindsight inspection reads on http://" + address)

	if err := hub.Run(); err != nil {
		panic(err)
	}
}
