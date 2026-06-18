package trader

import (
	"encoding/json"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/ui"
)

/*
measure applies one desk tick: collect tree readings, publish UI, walk playbooks.
*/
func (crypto *Crypto) measure() {
	scopes := crypto.collectMeasureScopes()

	crypto.ingestResonanceMeasurements(scopes)
	crypto.collectMeasurementsFromTree(scopes)
	crypto.settleStoryForwardFeedback()

	eventAt := time.Now()
	crypto.observeCognitiveMeasurements()
	readings := crypto.sealCognitiveScopes(scopes, eventAt)
	crypto.maybeConsolidateCognitive(eventAt)

	errnie.Error(ui.PublishMeasurements(
		crypto.pool,
		crypto.story.Measurements(),
		crypto.storyTicks.Add(1),
		crypto.story.PlaybookEvaluationCount(),
		crypto.story.AnchorWalkTrace(),
	))
	errnie.Error(crypto.publishCognitiveReadings(readings))

	if crypto.wallet != nil {
		errnie.Error(ui.PublishWallet(crypto.pool, crypto.wallet))
	}
}

/*
applyPlaybookActions evaluates story playbooks and routes fills through broker.
*/
func (crypto *Crypto) applyPlaybookActions() {
	if crypto.story == nil || crypto.desk == nil {
		return
	}

	holdings := crypto.storyHoldings()
	crypto.syncStoryBalances(holdings)

	for _, action := range sortActionsExitsFirst(crypto.story.Actions()) {
		if action == nil {
			continue
		}

		if !crypto.shouldSubmitPlaybookAction(action) {
			continue
		}

		errnie.Error(crypto.desk.SubmitAction(action, holdings))
	}
}

func (crypto *Crypto) shouldSubmitPlaybookAction(action *logic.Action) bool {
	if action.Type.IsExit() {
		return true
	}

	if crypto.memory == nil {
		return true
	}

	return !crypto.memory.Sideline(action.Symbol)
}

func (crypto *Crypto) storyHoldings() *logic.Balances {
	if crypto.wallet == nil {
		return nil
	}

	converted := userBalancesToLogic(crypto.wallet)

	return &converted
}

func (crypto *Crypto) settleStoryForwardFeedback() {
	if crypto == nil || crypto.story == nil || crypto.tree == nil {
		return
	}

	quotes := broker.NewQuoteCache(crypto.tree)
	crypto.story.SettleForwardFeedback(time.Now(), func(symbol string) (float64, bool) {
		quote, ok := quotes.QuoteForSymbol(symbol)

		if !ok {
			return 0, false
		}

		return quote.Mark()
	})
}

func (crypto *Crypto) syncStoryBalances(holdings *logic.Balances) {
	if crypto.story == nil {
		return
	}

	crypto.story.SetBalances(holdings)
}

/*
bootstrapWallet retries balances subscribe until the first snapshot hydrates wallet.
*/
func (crypto *Crypto) bootstrapWallet() {
	const (
		attempts = 20
		pause    = 50 * time.Millisecond
	)

	for attempt := 0; attempt < attempts; attempt++ {
		if crypto.wallet != nil {
			return
		}

		errnie.Error(crypto.subscribeBalances())
		time.Sleep(pause)
	}
}

/*
subscribeBalances requests the paper or live balances channel snapshot.
*/
func (crypto *Crypto) subscribeBalances() error {
	if crypto.pool == nil {
		return nil
	}

	message, buildErr := types.NewKrakenMessage("subscribe", user.BalanceParams{
		Channel:  "balances",
		Snapshot: true,
	}, 0)

	if buildErr != nil {
		return errnie.Error(buildErr)
	}

	payload, marshalErr := sonic.Marshal(message)

	if marshalErr != nil {
		return errnie.Error(marshalErr)
	}

	artifact := datura.Acquire("trader", datura.Artifact_Type_json).
		WithDestination("kraken:private").
		WithRole("balances").
		WithPayload(payload)

	return errnie.Error(
		crypto.pool.CreateBroadcastGroup("kraken:private").Send(artifact),
	)
}

func (crypto *Crypto) onBalancesMessage(artifact *datura.Artifact) error {
	if artifact == nil {
		return nil
	}

	payload := datura.As[user.Balances](artifact)
	crypto.wallet = &payload

	logicBalances := userBalancesToLogic(&payload)
	balanceArtifact := datura.Acquire("trader", datura.Artifact_Type_json).
		WithRole("balances")

	balancePayload, err := json.Marshal(logicBalances)

	if err != nil {
		return errnie.Error(err)
	}

	balanceArtifact.WithPayload(balancePayload)
	errnie.Error(crypto.story.Update(balanceArtifact))
	errnie.Error(ui.PublishWallet(crypto.pool, &payload))

	return nil
}

func userBalancesToLogic(balances *user.Balances) logic.Balances {
	if balances == nil {
		return logic.Balances{}
	}

	converted := logic.Balances{
		Inventory: make(map[string]float64),
		Asset:     make([]logic.BalanceAsset, 0, len(balances.Asset)),
	}

	if len(balances.Inventory) > 0 {
		for asset, quantity := range balances.Inventory {
			converted.Inventory[asset] = quantity
		}
	}

	quoteCurrency := balances.Currency

	for _, row := range balances.Asset {
		converted.Asset = append(converted.Asset, logic.BalanceAsset{
			Asset:   row.Asset,
			Balance: row.Balance,
		})

		if row.Asset == "" || row.Balance <= 0 {
			continue
		}

		if quoteCurrency != "" && row.Asset == quoteCurrency {
			continue
		}

		converted.Inventory[row.Asset] = row.Balance
	}

	return converted
}

func sortActionsExitsFirst(actions []*logic.Action) []*logic.Action {
	if len(actions) <= 1 {
		return actions
	}

	exits := make([]*logic.Action, 0, len(actions))
	entries := make([]*logic.Action, 0, len(actions))

	for _, action := range actions {
		if action == nil {
			continue
		}

		if action.Type.IsExit() {
			exits = append(exits, action)

			continue
		}

		entries = append(entries, action)
	}

	return append(exits, entries...)
}
