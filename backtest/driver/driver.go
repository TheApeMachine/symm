package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/backtest"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
State is the driver's broadcast snapshot: which capture is loaded, where
playback stands, and the capture's time bounds, which frame the scrub slider.
*/
type State struct {
	CaptureID int64     `json:"captureId"`
	Playing   bool      `json:"playing"`
	Position  time.Time `json:"position"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
	Rebooting bool      `json:"rebooting"`
}

/*
Driver runs captured sessions through the full production stack in-process:
the same boot, the same signals, the same desk — only the transports are
fixture connections fed from the capture store, and the clock is the capture's
own arrival times. The dashboard receives exactly the wire frames a live run
would produce.

Playback is commanded, never automatic: Play releases the pump, Pause parks
it, and Seek rebuilds the whole stack and fast-forwards (unpaced) to the
target time before holding — every stage from books to baselines is stateful,
so only a fresh boot replays history honestly.
*/
type Driver struct {
	ctx     context.Context
	store   *backtest.Store
	ui      *transport.MapReduce[[]byte]
	hub     *ui.Hub
	onState func(State)

	stateMu sync.Mutex
	state   State

	commands         chan command
	captures         []backtest.CaptureInfo
	hindsightRunning atomic.Int64
}

type command struct {
	kind      string
	at        time.Time
	captureID int64
}

/*
NewDriver creates the replay supervisor for one sqlite capture store.
*/
func NewDriver(
	ctx context.Context,
	store *backtest.Store,
	hub *ui.Hub,
	ui *transport.MapReduce[[]byte],
	onState func(State),
) *Driver {
	captures, err := store.ListCaptures()

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "backtest: list captures", err))
	}

	driver := &Driver{
		ctx:      ctx,
		store:    store,
		hub:      hub,
		ui:       ui,
		onState:  onState,
		commands: make(chan command, 8),
		state:    State{},
		captures: captures,
	}

	go driver.supervise()

	return driver
}

/*
Play resumes playback from the held position.
*/
func (driver *Driver) Play() {
	driver.send(command{kind: "play"})
}

/*
Pause parks the pump at the current position.
*/
func (driver *Driver) Pause() {
	driver.send(command{kind: "pause"})
}

/*
Seek rebuilds the stack and fast-forwards to the given capture time.
*/
func (driver *Driver) Seek(at time.Time) {
	driver.send(command{kind: "seek", at: at})
}

/*
Select loads a different capture and holds at its start.
*/
func (driver *Driver) Select(captureID int64) {
	driver.send(command{kind: "select", captureID: captureID})
}

/*
Snapshot returns the current playback state.
*/
func (driver *Driver) Snapshot() State {
	driver.stateMu.Lock()
	defer driver.stateMu.Unlock()

	return driver.state
}

func (driver *Driver) send(next command) {
	select {
	case driver.commands <- next:
	case <-driver.ctx.Done():
	}
}

func (driver *Driver) update(apply func(*State)) {
	driver.stateMu.Lock()
	apply(&driver.state)
	current := driver.state
	driver.stateMu.Unlock()

	if driver.onState != nil {
		driver.onState(current)
	}
}

func (driver *Driver) silentUpdate(apply func(*State)) {
	driver.stateMu.Lock()
	apply(&driver.state)
	driver.stateMu.Unlock()
}

/*
supervise owns the one live session. Every select or seek tears the session
down and boots a fresh stack; play and pause only toggle the pump.
*/
func (driver *Driver) supervise() {
	holdAt := time.Time{}
	captureID := int64(0)

	if len(driver.captures) > 0 {
		captureID = driver.captures[0].ID
	}

	for {
		if captureID != 0 {
			driver.runSession(captureID, holdAt)
		}

		select {
		case <-driver.ctx.Done():
			return
		case next := <-driver.commands:
			switch next.kind {
			case "select":
				captureID = next.captureID
				holdAt = time.Time{}

				// Hindsight is cheap to start and runs on its own store
				// connection, so every capture selection refreshes the
				// perfect-execution panel for that tape.
				driver.Hindsight(captureID)
			case "seek":
				holdAt = next.at
			case "play":
				driver.update(func(state *State) { state.Playing = true })

				continue
			case "pause":
				driver.update(func(state *State) { state.Playing = false })

				continue
			}
		}
	}
}

/*
runSession boots one full stack over the capture and pumps frames until the
capture ends, the session is replaced, or playback is parked at the hold
position.
*/
func (driver *Driver) runSession(captureID int64, holdAt time.Time) {
	startedAt, endedAt, err := driver.store.Bounds(captureID)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "backtest: load capture", err))
		return
	}

	symbols, err := driver.captureSymbols(captureID)

	if err != nil || len(symbols) == 0 {
		errnie.Error(errnie.Err(errnie.Internal, "backtest: capture has no symbols", err))
		return
	}

	driver.update(func(state *State) {
		state.CaptureID = captureID
		state.Rebooting = true
		state.Playing = false
		state.Position = holdAt
		state.StartedAt = startedAt
		state.EndedAt = endedAt
	})

	sessionCtx, cancel := context.WithCancel(driver.ctx)
	defer cancel()

	previousModel := viper.Get("trading.model")
	viper.Set("trading.model", "real")

	// The fixture venue answers instantly; the live subscription pacing is
	// politeness to Kraken and would tax every session boot seconds per
	// batch. The stack boots as fast as the fixtures stream.
	previousPace := viper.Get("market.subscribe.pace")
	viper.Set("market.subscribe.pace", time.Millisecond)
	restoreAuth := fixtureAuth()

	config := testtypes.NewScenarioConfig(symbols)
	config.StartTime = startedAt
	config.InitialBalances = map[string]float64{"USD": 200}
	config.Execution.DepthLevels = 10

	market, marketErr := tests.NewMarketWithScenario(sessionCtx, config)

	if marketErr != nil {
		restoreAuth()
		viper.Set("trading.model", previousModel)
		errnie.Error(errnie.Err(errnie.Internal, "backtest: build market", marketErr))

		return
	}

	market.WithAutoFill(config.Execution)

	publicFeed, privateFeed := market.Feeds()
	system := cmd.BootWithHub(
		sessionCtx, types.NewThesis(sessionCtx, driver.ui),
		publicFeed, privateFeed, driver.ui, driver.hub,
	)

	if system == nil {
		restoreAuth()
		viper.Set("trading.model", previousModel)
		viper.Set("market.subscribe.pace", previousPace)
		return
	}

	market.Drive(system)

	defer func() {
		restoreAuth()
		viper.Set("trading.model", previousModel)
		viper.Set("market.subscribe.pace", previousPace)
		_ = system.Close()
	}()

	driver.update(func(state *State) {
		state.Rebooting = false
		state.Playing = holdAt.IsZero()
	})

	from := holdAt

	if from.IsZero() {
		from = startedAt
	}

	frames, release, err := driver.store.Frames(captureID, from)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "backtest: open frames", err))
		return
	}

	defer release()

	fastForward := !holdAt.IsZero()
	previousAt := from

	paused := make(chan struct{})

	if !driver.Snapshot().Playing {
		close(paused)
	}

	for {
		select {
		case <-sessionCtx.Done():
			return
		case next := <-driver.commands:
			switch next.kind {
			case "pause":
				select {
				case <-paused:
				default:
					close(paused)
				}

				driver.update(func(state *State) { state.Playing = false })

				continue
			case "play":
				select {
				case <-paused:
					paused = make(chan struct{})
				default:
				}

				driver.update(func(state *State) { state.Playing = true })

				continue
			default:
				driver.send(next)
				return
			}
		default:
		}

		frame, ok := frames()

		if !ok {
			driver.update(func(state *State) {
				state.Playing = false
				state.Position = endedAt
			})

			driver.awaitCommand()

			return
		}

		select {
		case <-paused:
			driver.silentUpdate(func(state *State) { state.Position = frame.ReceivedAt })

			<-paused

			paused = make(chan struct{})
		default:
		}

		if !fastForward && frame.ReceivedAt.After(previousAt) {
			select {
			case <-time.After(frame.ReceivedAt.Sub(previousAt)):
			case <-sessionCtx.Done():
				return
			}
		}

		driver.publishFrame(market, frame)
		previousAt = frame.ReceivedAt

		driver.silentUpdate(func(state *State) { state.Position = frame.ReceivedAt })
	}
}

/*
publishFrame routes one captured payload to the fixture connection the live
transport would have received it on, byte-for-byte.
*/
func (driver *Driver) publishFrame(market *tests.Market, frame backtest.Frame) {
	channel := wireChannel(frame.Payload)

	switch frame.Endpoint {
	case "level3":
		market.Level3.Publish(channel, frame.Payload)
	case "public":
		market.Public.Publish(channel, frame.Payload)
	case "private":
		market.Private.Publish(channel, frame.Payload)
	}
}

/*
captureSymbols pre-scans one capture's ticker frames for the traded universe
and a starting price per symbol, which the fixture venue needs to configure
instruments and execution pricing.
*/
func (driver *Driver) captureSymbols(captureID int64) ([]*testtypes.Symbol, error) {
	frames, release, err := driver.store.Frames(captureID, time.Time{})

	if err != nil {
		return nil, err
	}

	defer release()

	prices := map[string]float64{}
	scanned := 0
	const symbolScanLimit = 20000

	for {
		if scanned >= symbolScanLimit {
			break
		}

		frame, ok := frames()

		if !ok {
			break
		}

		scanned++

		if frame.Endpoint != "public" || channelIsNot(frame.Payload, "ticker") {
			continue
		}

		var ticker struct {
			Data []struct {
				Symbol string  `json:"symbol"`
				Last   float64 `json:"last"`
			} `json:"data"`
		}

		if err := json.Unmarshal(frame.Payload, &ticker); err != nil {
			continue
		}

		for _, row := range ticker.Data {
			if row.Symbol != "" && row.Last > 0 {
				prices[row.Symbol] = row.Last
			}
		}

		if len(prices) >= 512 {
			break
		}
	}

	symbols := make([]*testtypes.Symbol, 0, len(prices))
	index := int64(1)

	for pair, price := range prices {
		symbols = append(symbols, testtypes.NewSymbol(pair, price, index))
		index++
	}

	errnie.Info(fmt.Sprintf(
		"backtest: capture %d spans %d symbols", captureID, len(symbols),
	))

	return symbols, nil
}

func (driver *Driver) awaitCommand() {
	select {
	case <-driver.ctx.Done():
	case next := <-driver.commands:
		driver.send(next)
	}
}

/*
fixtureAuth mirrors the test fixtures' venue credentials; the transport's
authenticate path reads the environment before the fixture answers it.
*/
func fixtureAuth() func() {
	previousKey, previousSecret := os.Getenv("KRAKEN_API_KEY"), os.Getenv("KRAKEN_API_SECRET")
	os.Setenv("KRAKEN_API_KEY", "fixture-key")
	os.Setenv("KRAKEN_API_SECRET", "Zml4dHVyZS1zZWNyZXQ=")

	return func() {
		os.Setenv("KRAKEN_API_KEY", previousKey)
		os.Setenv("KRAKEN_API_SECRET", previousSecret)
	}
}

func wireChannel(payload []byte) string {
	var header struct {
		Channel string `json:"channel"`
	}

	if err := json.Unmarshal(payload, &header); err != nil {
		return ""
	}

	return header.Channel
}

func channelIsNot(payload []byte, channel string) bool {
	return wireChannel(payload) != channel
}
