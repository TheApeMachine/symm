# do-it-right

These are the FACTS:

1. You constantly severely over-engineer everything
2. You write mostly slop code, and that is unacceptable
3. You have ruined this project and now you are going to fix it, correctly

You will strictly adhere to the following RULES:

1. You do not write any new abstractions without explicit permission
2. By default you do not have permission
3. You may ONLY use the numeric packages
4. You will put an effort to find the most compact, yet correct implementation, using numeric composition

THE CORRECT SYSTEM ARCHITECTURE:

1. Everything MUST be a System

    type System interface {
        Start() error
        State() State
        Tick() error
        Close() error
    }

2. All systems must be registered in root.go

    booter.AddSystems(
        client.NewPublicClient(cmd.Context(), pool, "wss://ws.kraken.com/v2"),
        pumpdump.NewPumpDump(cmd.Context(), pool),
        trader.NewCrypto(cmd.Context(), pool, trader.NewWallet(
            trader.PaperWallet, config.System.QuoteCurrency, config.System.WalletEUR, config.System.TakerFeePct,
        )),
    )

3. All systems use the same Tick() shape:

    func (publicClient *PublicClient) Tick() error {
        select {
        case <-publicClient.ctx.Done():
            publicClient.cancel()
            return publicClient.ctx.Err()
        case msg := <-publicClient.subscribers["subscriptions"].Incoming:
            if msg, ok := msg.Value.([]string); !ok {
                return errnie.Error(fmt.Errorf("invalid subscriptions message: %v", msg))
            }

            for _, symbol := range msg.Value.([]string) {
                subscription := errnie.Does(func() (*ohlc.Subscribe, error) {
                    return ohlc.NewSubscribe([]string{symbol}), nil
                }).Or(func(err error) {
                    errnie.Error(err)
                }).Value()

                publicClient.conn.WriteJSON(subscription)
            }

            return nil
        default:
            return nil
        }
    }

4. You will avoid excessive helper methods
5. You must never have something that is reusable as a specific method on a type, keep things compact
6. Always use the qpool, broadcast groups, and subscribers to communicate/send data, and do not add any abstraction
7. If something was deleted and is no longer there, it should not come back, think of a way to do it with what you have available, or remove it if it isn't actually useful.

8. If you need to send data to the UI/Frontend:

    privateClient.broadcasts["ui"].Send(&qpool.QValue[any]{
        Value: payload,
    })

9. NOTHING IS OUT OF SCOPE AND YOU DO NOT MAKE THAT DECISION, EVER. DO THE WORK.

10. Model the Kraken data correctly:

    package ohlc

    import "time"

    type Data struct {
        Symbol        string    `json:"symbol"`
        Open          float64   `json:"open"`
        High          float64   `json:"high"`
        Low           float64   `json:"low"`
        Close         float64   `json:"close"`
        VWAP          float64   `json:"vwap"`
        Volume        float64   `json:"volume"`
        IntervalBegin time.Time `json:"count"`
        Interval      time.Time `json:"interval"`
    }

    type Snapshot struct {
        Channel string `json:"channel"`
        Type    string `json:"type"`
        Data    []Data `json:"data"`
    }

    package ohlc

    import "time"

    type Params struct {
        Channel  string   `json:"channel"`
        Symbol   []string `json:"symbol"`
        Interval int      `json:"interval"`
        Snapshot bool     `json:"snapshot"`
    }

    type Subscribe struct {
        Method string `json:"method"`
        Params any    `json:"params"`
        ReqID  int    `json:"req_id,omitempty"`
    }

    func NewSubscribe(symbols []string) *Subscribe {
        return &Subscribe{
            Method: "subscribe",
            Params: Params{
                Channel:  "ohlc",
                Symbol:   symbols,
                Interval: 1,
                Snapshot: true,
            },
            ReqID: int(time.Now().UnixNano()),
        }
    }

And you don't have to make all kinds of special types for that, just send the data as soon as you have any, right at the source, no "snapshots" no "drains" just simple.

THE OVERALL SYSTEM WORKS LIKE THIS:

1. Signals convert raw market data into Measurement
   Each Signal MUST emit a Measurement on each Tick, which includes a CONFIDENCE
   Each Signal decides for themselves which asset pairs to subscribe to, and EVERY SUBSCRIBED ASSET PAIR SHOULD SEND ITS PRICE TO THE FRONTEND FROM THE SOURCE (public Kraken websocket connection), THIS IS ALSO TRUE FOR EVERY ACTIVE TRADE THOUGH THAT SHOULD ALREADY BE SUBSCRIBED TO SINCE THE SIGNALS ADVICE THE TRADES.
2. Measurements are picked up by the trader
   The trader takes the running average of the CONFIDENCE of each Signal and sends this to the UI for the GAUGES
3. The trader combines Measurements into !!!Perspectives!!!
4. The trader !!!uses Perspectives to make Predictions!!! (**always, not just when entering a trade**)
5. Predictions are evaluated once current time has caught up **THEY ARE PREDICTIONS IN TIME, NOT CYCLES, SO CHART ALSO HAS TO REFLECT THAT**
6. **!!!The error of the Prediction is used as top-down feedback to modulate the paramters/values the Signals that were part of the Perspective that makde the prediction use!!!**

AND FINALLY MAKE ABSOLUTELY SURE THAT ALL THE CHARTS ON THE FRONTEND SHOW ACCURATE DATA AND EVERYTHING THAT NEEDS DATA ALSO RECEIVES IT!!!!!

AND NEVER EVER RESTORE ANYTHING FROM GIT THAT IS BACKWARDS NOT FORWARDS, TAKE THE SYSTEM AS IT IS.

(Oh and check the Makefile for the linker error)