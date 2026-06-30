package trader

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/resonance"
)

type SystemState uint8

const (
	PREPARING SystemState = iota
	READY
)

/*
Crypto orchestrates measurement collection, playbook walks, and broker fills.
*/
type Crypto struct {
	ctx          context.Context
	cancel       context.CancelFunc
	state        SystemState
	tree         *dmt.Tree
	pool         *qpool.Q[any]
	uiBroadcast  *qpool.BroadcastGroup
	broadcasts   *sync.Map
	subscribers  *sync.Map
	desk         *broker.Desk
	story        *market.Story
	balanceMu    sync.RWMutex
	balances     *datura.Artifact
	signals      *Signal
	crossSection *market.CrossSection
	resonance    *resonance.Signal
	allocator    *Allocator
	decider      *Decider
	audit        *audit.Recorder
	tick         *atomic.Int64
}

func NewCrypto(
	ctx context.Context, pool *qpool.Q[any], tree *dmt.Tree,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	crossSection, err := market.NewCrossSection(
		market.DefaultCrossSectionConfig(),
	)

	if err != nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: failed to create cross section",
			err,
		))
	}

	signals := NewSignal(ctx, pool, tree)

	resonanceSignal := resonance.NewSignal(
		ctx,
		pool,
		tree,
		viper.GetIntSlice("signals.resonance.arch"),
		viper.GetFloat64("signals.resonance.alpha"),
		viper.GetInt("signals.resonance.batch"),
	)

	auditRecorder, err := newCryptoAuditRecorder()
	if err != nil {
		cancel()
		return nil, err
	}

	crypto := &Crypto{
		ctx:          ctx,
		cancel:       cancel,
		state:        PREPARING,
		tree:         tree,
		pool:         pool,
		uiBroadcast:  pool.CreateBroadcastGroup("ui"),
		broadcasts:   &sync.Map{},
		subscribers:  &sync.Map{},
		desk:         broker.NewDesk(ctx, pool, tree),
		story:        market.NewStory(ctx, pool),
		signals:      signals,
		crossSection: crossSection,
		resonance:    resonanceSignal,
		allocator:    NewAllocator(),
		decider:      NewDecider(tree),
		audit:        auditRecorder,
		tick:         &atomic.Int64{},
	}

	for _, channel := range []string{"kraken:public"} {
		crypto.broadcasts.Store(
			channel,
			crypto.pool.CreateBroadcastGroup(channel),
		)
	}

	for _, channel := range []string{"balances"} {
		crypto.subscribers.Store(
			channel, pool.Subscribe(channel, crypto.onMessage),
		)
	}

	return crypto, nil
}

func newCryptoAuditRecorder() (*audit.Recorder, error) {
	if !auditEnabled() {
		return nil, nil
	}

	filename := strings.TrimSpace(viper.GetString("trading.audit.file"))
	if filename == "" {
		filename = strings.TrimSpace(viper.GetString("system.audit.file"))
	}
	if filename == "" {
		return nil, nil
	}

	recorder, err := audit.NewRecorder(filename)
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"trader: failed to open audit recorder",
			err,
		))
	}

	return recorder, nil
}

func auditEnabled() bool {
	if viper.IsSet("trading.audit.file") && strings.TrimSpace(viper.GetString("trading.audit.file")) != "" {
		return true
	}

	return viper.GetBool("system.audit.enabled")
}

/*
onMessage is called by the qpool.BroadcastGroup for every consumer
that has subscribed with a callback function.
*/
func (crypto *Crypto) onMessage(
	artifact *datura.Artifact,
) error {
	role := datura.Peek[string](artifact, "role")

	switch role {
	case "balances":
		if len(balanceRows(artifact)) == 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"trader: balances artifact missing data",
				nil,
			))
		}

		crypto.setBalances(artifact)
		crypto.uiBroadcast.Send(artifact)
	}

	return nil
}

func (crypto *Crypto) setBalances(balances *datura.Artifact) {
	crypto.balanceMu.Lock()
	defer crypto.balanceMu.Unlock()
	crypto.balances = balances
}

func (crypto *Crypto) Balances() *datura.Artifact {
	crypto.balanceMu.RLock()
	defer crypto.balanceMu.RUnlock()
	return crypto.balances
}

func (crypto *Crypto) Run() error {
	// The loop is paced by the data, not a fixed wall-clock interval: after each
	// pass it sleeps for the cadence the signals actually observed (bounded).
	// Empty passes still emit a tick heartbeat so the frontend can distinguish
	// a quiet market from a dead trader loop.
	timer := time.NewTimer(crypto.signals.PollInterval())
	defer timer.Stop()

	for crypto.state != READY {
		time.Sleep(1 * time.Second)

		if errnie.Require(map[string]any{
			"ctx":          crypto.ctx,
			"cancel":       crypto.cancel,
			"tree":         crypto.tree,
			"pool":         crypto.pool,
			"uiBroadcast":  crypto.uiBroadcast,
			"broadcasts":   crypto.broadcasts,
			"subscribers":  crypto.subscribers,
			"desk":         crypto.desk,
			"story":        crypto.story,
			"signals":      crypto.signals,
			"crossSection": crypto.crossSection,
			"resonance":    crypto.resonance,
			"allocator":    crypto.allocator,
			"decider":      crypto.decider,
			"balances":     crypto.Balances(),
		}) == nil {
			crypto.state = READY
		}
	}

	errnie.Info("crypto trader ready")

	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		case <-timer.C:
			timer.Reset(crypto.signals.PollInterval())
			tickCount := crypto.tick.Add(1)

			// Build the peer snapshot once per tick before measuring, so every
			// signal reads a complete, consistent cross-section.
			crypto.signals.Observe(crypto.crossSection)

			resonanceMeasurements := crypto.signals.MeasureResonance(crypto.resonance)
			measurements := crypto.signals.Measure(crypto.crossSection)
			measurements = append(measurements, resonanceMeasurements...)
			cognitiveReadings := market.CognitiveReadings(crypto.tree, measurements)
			market.ApplyCognitiveReadings(measurements, cognitiveReadings)
			balances := crypto.Balances()
			crypto.story.Update(measurements)
			actions := crypto.story.Actions(balances)
			chosen, verdicts := crypto.decider.choose(measurements, actions, balances)
			crypto.allocator.SetPendingEntries(crypto.desk.PendingEntryCount())
			allowed := crypto.allocator.Allowed(chosen, balances)
			if len(allowed) > 0 {
				errnie.Error(crypto.desk.Update(allowed))
			}
			openPositions := crypto.publishPositions(tickCount)
			crypto.publishManifoldSnapshot(tickCount, measurements)
			crypto.publishRegimeSnapshot(tickCount, measurements)

			if len(cognitiveReadings) > 0 {
				crypto.uiBroadcast.Send(datura.Acquire(
					"trader", datura.APPJSON,
				).WithDestination(
					"ui",
				).WithRole(
					"cognitive",
				).WithScope(
					"cognitive",
				).WithPayload(datura.Map[any]{
					"tick":     tickCount,
					"readings": cognitiveReadings,
				}.Marshal()))
			}

			for _, measurement := range measurements {
				measurement.WithAttribute("journey.trader.tick", tickCount)
				crypto.uiBroadcast.Send(measurement.WithDestination("ui"))
			}

			for _, verdict := range verdicts {
				if verdict.action == nil {
					continue
				}

				crypto.uiBroadcast.Send(verdict.action.WithDestination("ui"))
			}

			for _, action := range allowed {
				if datura.Peek[string](action, "verdict") != "" {
					continue
				}

				crypto.uiBroadcast.Send(action.WithDestination("ui"))
			}

			crypto.publishDecisions(tickCount, verdicts, allowed)

			crypto.uiBroadcast.Send(datura.Acquire(
				"trader", datura.APPJSON,
			).WithDestination(
				"ui",
			).WithRole(
				"tick",
			).WithScope(
				"tick",
			).WithPayload(datura.Map[any]{
				"count":        tickCount,
				"tick":         tickCount,
				"phase":        "stream",
				"candidates":   len(actions),
				"open":         openPositions,
				"quotes_ready": crypto.signals.RoleCount("ticker"),
				"quotes_total": crypto.quotesTotal(measurements),
				"fluid":        originCount(measurements, string(logic.SourceFluid)),
			}.Marshal()))
		}
	}
}

func (crypto *Crypto) publishDecisions(
	tickCount int64,
	verdicts []verdict,
	allowed []*datura.Artifact,
) {
	bySymbol := make(map[string]datura.Map[any])

	for _, verdict := range verdicts {
		crypto.mergeDecisionRecord(bySymbol, tickCount, verdict.action, verdict.reason)
	}

	for _, action := range allowed {
		crypto.mergeDecisionRecord(bySymbol, tickCount, action, "")
	}

	decisions := make([]datura.Map[any], 0, len(bySymbol))
	for _, decision := range bySymbol {
		decisions = append(decisions, decision)
	}

	sort.Slice(decisions, func(first, second int) bool {
		return decisionScore(decisions[first]) > decisionScore(decisions[second])
	})

	frame := datura.Map[any]{
		"tick":      tickCount,
		"decisions": decisions,
	}

	crypto.writeDecisionAudit(frame)

	crypto.uiBroadcast.Send(datura.Acquire(
		"trader", datura.APPJSON,
	).WithDestination(
		"ui",
	).WithRole(
		"decisions",
	).WithScope(
		"decisions",
	).WithPayload(frame.Marshal()))
}

func (crypto *Crypto) writeDecisionAudit(frame datura.Map[any]) {
	if crypto == nil || crypto.audit == nil || len(frame) == 0 {
		return
	}

	decisions, ok := frame["decisions"].([]datura.Map[any])
	if ok && len(decisions) == 0 {
		return
	}
	if !ok {
		errnie.Error(crypto.audit.Write(frame))
		return
	}

	for _, decision := range decisions {
		row := datura.Map[any]{
			"channel":   "diagnostic",
			"type":      "decision",
			"severity":  decisionSeverity(decision),
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		}
		if tick, tickOK := frame["tick"]; tickOK {
			row["tick"] = tick
		}
		for key, value := range decision {
			row[key] = value
		}

		if row["reason"] == nil {
			row["reason"] = row["why"]
		}

		errnie.Error(crypto.audit.Write(row))
	}
}

func decisionSeverity(decision datura.Map[any]) string {
	verdict, _ := decision["verdict"].(string)
	switch verdict {
	case "allow", "admitted":
		return "info"
	default:
		return "warning"
	}
}

func (crypto *Crypto) mergeDecisionRecord(
	bySymbol map[string]datura.Map[any],
	tickCount int64,
	action *datura.Artifact,
	fallbackReason string,
) {
	if action == nil {
		return
	}

	symbol, _ := action.Scope()
	if symbol == "" {
		symbol = datura.Peek[string](action, "symbol")
	}
	if symbol == "" {
		return
	}

	verdict := datura.Peek[string](action, "verdict")
	if verdict == "" {
		verdict = datura.Peek[string](action, "decision", "verdict")
	}
	if verdict == "" && datura.Peek[bool](action, "allowed") {
		verdict = "allow"
	}
	if verdict == "" {
		verdict = "blocked"
	}

	reason := datura.Peek[string](action, "why")
	if reason == "" {
		reason = datura.Peek[string](action, "decision", "reason")
	}
	if reason == "" {
		reason = fallbackReason
	}

	role, _ := action.Role()
	record := datura.Map[any]{
		"tick":                tickCount,
		"symbol":              symbol,
		"side":                datura.Peek[string](action, "side"),
		"type":                datura.Peek[string](action, "type"),
		"role":                role,
		"source":              datura.Peek[string](action, "reason_source"),
		"category":            datura.Peek[string](action, "reason_category"),
		"verdict":             verdict,
		"why":                 reason,
		"score":               decisionArtifactScore(action),
		"confidence":          datura.Peek[float64](action, "decision", "confidence"),
		"edge":                datura.Peek[float64](action, "decision", "edge"),
		"hurdle":              datura.Peek[float64](action, "decision", "hurdle"),
		"friction":            datura.Peek[float64](action, "decision", "friction"),
		"expected_return_bps": datura.Peek[float64](action, "decision", "expected_return_bps"),
		"friction_bps":        datura.Peek[float64](action, "decision", "friction_bps"),
		"net_edge_bps":        datura.Peek[float64](action, "decision", "net_edge_bps"),
		"sample_count":        datura.Peek[int](action, "decision", "sample_count"),
		"calibration_ready":   datura.Peek[bool](action, "decision", "calibration_ready"),
		"edge_source":         datura.Peek[string](action, "decision", "edge_source"),
		"fraction":            datura.Peek[float64](action, "fraction"),
		"notional":            datura.Peek[float64](action, "notional"),
		"allowed":             datura.Peek[bool](action, "allowed"),
	}
	if priced := datura.Peek[bool](action, "decision", "economic_priced"); priced {
		record["economic_priced"] = priced
	}
	if liquidity := datura.Peek[string](action, "execution", "liquidity"); liquidity != "" {
		record["liquidity"] = liquidity
	}

	if record["side"] == "" {
		record["side"] = role
	}
	if record["source"] == "" {
		record["source"] = datura.Peek[string](action, "source")
	}
	if record["confidence"] == 0.0 {
		record["confidence"] = datura.Peek[float64](action, "confidence")
	}

	prior, exists := bySymbol[symbol]
	if !exists || preferDecisionRecord(record, prior) {
		bySymbol[symbol] = record
	}
}

func preferDecisionRecord(next, prior datura.Map[any]) bool {
	nextAllowed := next["verdict"] == "allow" || next["allowed"] == true
	priorAllowed := prior["verdict"] == "allow" || prior["allowed"] == true

	if nextAllowed != priorAllowed {
		return nextAllowed
	}

	return decisionScore(next) > decisionScore(prior)
}

func decisionArtifactScore(action *datura.Artifact) float64 {
	score := datura.Peek[float64](action, "decision", "score")
	if score > 0 {
		return score
	}

	return datura.Peek[float64](action, "score")
}

func decisionScore(decision datura.Map[any]) float64 {
	score, _ := decision["score"].(float64)

	return score
}

func (crypto *Crypto) publishManifoldSnapshot(
	tickCount int64,
	measurements []*datura.Artifact,
) {
	if crypto == nil || crypto.signals == nil || crypto.uiBroadcast == nil {
		return
	}

	payload, snapshotErr := crypto.signals.FieldSnapshot(latestMeasurementTime(measurements))
	if snapshotErr != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: manifold snapshot failed",
			snapshotErr,
		))

		return
	}

	if len(payload) == 0 {
		return
	}

	payload["tick"] = tickCount

	marshaled, marshalErr := sonic.Marshal(payload)
	if marshalErr != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: marshal manifold snapshot",
			marshalErr,
		))

		return
	}

	artifact := datura.Acquire(
		"trader", datura.APPJSON,
	).WithDestination(
		"ui",
	).WithRole(
		"manifold",
	).WithScope(
		"manifold",
	)

	if artifact.WithPayload(marshaled) == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: manifold snapshot payload rejected",
			nil,
		))

		return
	}

	crypto.uiBroadcast.Send(artifact)
}

type regimeAxisSource struct {
	origin     logic.SourceType
	categories []logic.CategoryType
}

var regimeAxisSources = map[string][]regimeAxisSource{
	"volatility": {
		{origin: logic.SourceFluid, categories: []logic.CategoryType{
			logic.CategoryTurbulent,
			logic.CategoryInertial,
		}},
		{origin: logic.SourceCorrelation, categories: []logic.CategoryType{
			logic.CategoryDivergentStress,
		}},
		{origin: logic.SourceManifold, categories: []logic.CategoryType{
			logic.CategoryLiquidityShock,
		}},
	},
	"trend": {
		{origin: logic.SourcePumpDump, categories: []logic.CategoryType{
			logic.CategoryVerticalIgnition,
			logic.CategoryOrganicTrend,
		}},
		{origin: logic.SourceHawkes, categories: []logic.CategoryType{
			logic.CategoryFrenzy,
			logic.CategoryOrganic,
		}},
		{origin: logic.SourceCVD, categories: []logic.CategoryType{
			logic.CategoryAggressiveDrive,
		}},
	},
	"bullish": {
		{origin: logic.SourceSentiment, categories: []logic.CategoryType{
			logic.CategoryRiskOnSurge,
		}},
		{origin: logic.SourceToxicity, categories: []logic.CategoryType{
			logic.CategoryHardSupport,
		}},
		{origin: logic.SourceLiquidity, categories: []logic.CategoryType{
			logic.CategoryRobustLiquidity,
		}},
	},
	"bearish": {
		{origin: logic.SourceSentiment, categories: []logic.CategoryType{
			logic.CategorySystemicSlump,
		}},
		{origin: logic.SourceToxicity, categories: []logic.CategoryType{
			logic.CategoryLiquidityVacuum,
			logic.CategoryToxicBluff,
		}},
		{origin: logic.SourceExhaustion, categories: []logic.CategoryType{
			logic.CategoryMechanicalCollapse,
			logic.CategoryThermalExhaustion,
		}},
	},
	"choppiness": {
		{origin: logic.SourcePumpDump, categories: []logic.CategoryType{
			logic.CategoryCoiledCompression,
		}},
		{origin: logic.SourceCVD, categories: []logic.CategoryType{
			logic.CategoryStochasticBalance,
		}},
		{origin: logic.SourceCorrelation, categories: []logic.CategoryType{
			logic.CategoryStochasticNoise,
		}},
		{origin: logic.SourceHawkes, categories: []logic.CategoryType{
			logic.CategorySaturation,
		}},
	},
}

func (crypto *Crypto) publishRegimeSnapshot(
	tickCount int64,
	measurements []*datura.Artifact,
) {
	if crypto == nil || crypto.uiBroadcast == nil {
		return
	}

	payload := regimeSnapshotPayload(tickCount, measurements)
	if payload == nil {
		return
	}

	crypto.uiBroadcast.Send(datura.Acquire(
		"regime", datura.APPJSON,
	).WithDestination(
		"ui",
	).WithRole(
		"regime",
	).WithScope(
		"regime",
	).WithPayload(payload.Marshal()))
}

func regimeSnapshotPayload(
	tickCount int64,
	measurements []*datura.Artifact,
) datura.Map[any] {
	samples := 0

	for _, measurement := range measurements {
		if measurement != nil {
			samples++
		}
	}

	if samples == 0 {
		return nil
	}

	payload := datura.Map[any]{
		"tick":    tickCount,
		"samples": samples,
	}

	total := 0.0
	peak := 0.0
	axes := 0.0
	for axis, sources := range regimeAxisSources {
		value := regimeAxisValue(measurements, sources)
		payload[axis] = value
		total += value
		axes++
		if value > peak {
			peak = value
		}
	}

	confidence := 0.0
	if axes > 0 {
		confidence = total / axes
	}

	payload["output"] = datura.Map[any]{
		"confidence": confidence,
		"strength":   peak,
		"status":     "measured",
		"volatility": payload["volatility"],
		"trend":      payload["trend"],
		"bullish":    payload["bullish"],
		"bearish":    payload["bearish"],
		"choppiness": payload["choppiness"],
	}

	return payload
}

func regimeAxisValue(
	measurements []*datura.Artifact,
	sources []regimeAxisSource,
) float64 {
	if len(measurements) == 0 || len(sources) == 0 {
		return 0
	}

	total := 0.0
	count := 0

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		origin, err := measurement.Origin()
		if err != nil || origin == "" {
			continue
		}

		for _, source := range sources {
			if origin != string(source.origin) {
				continue
			}

			total += regimeCategoryMass(measurement, source.categories)
			count++
			break
		}
	}

	if count == 0 {
		return 0
	}

	return total / float64(count)
}

func regimeCategoryMass(
	measurement *datura.Artifact,
	categories []logic.CategoryType,
) float64 {
	mass := 0.0

	for _, category := range categories {
		mass += datura.Peek[float64](
			measurement,
			"output",
			fmt.Sprintf("category.%d", logic.CategoryIndex(category)),
		)
	}

	if mass < 0 {
		return 0
	}

	if mass > 1 {
		return 1
	}

	return mass
}

func latestMeasurementTime(measurements []*datura.Artifact) time.Time {
	latest := int64(0)

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		if stamp := measurement.Timestamp(); stamp > latest {
			latest = stamp
		}
	}

	if latest <= 0 {
		return time.Now().UTC()
	}

	return time.Unix(0, latest).UTC()
}

func (crypto *Crypto) publishPositions(tickCount int64) int {
	positions, err := market.PositionReadings(
		crypto.tree,
		viper.GetString("market.quote_currency"),
	)

	if err != nil {
		errnie.Error(err)
		return 0
	}

	if positions == nil {
		return 0
	}

	crypto.uiBroadcast.Send(datura.Acquire(
		"trader", datura.APPJSON,
	).WithDestination(
		"ui",
	).WithRole(
		"positions",
	).WithScope(
		"positions",
	).WithPayload(datura.Map[any]{
		"tick":      tickCount,
		"quote":     strings.ToUpper(viper.GetString("market.quote_currency")),
		"positions": positions,
	}.Marshal()))

	return len(positions)
}

func (crypto *Crypto) quotesTotal(measurements []*datura.Artifact) int {
	if ready := crypto.signals.RoleCount("ticker"); ready > 0 {
		return ready
	}

	scopes := make(map[string]struct{})
	for _, measurement := range measurements {
		scope, err := measurement.Scope()
		if err == nil && scope != "" {
			scopes[scope] = struct{}{}
		}
	}

	return len(scopes)
}

func originCount(measurements []*datura.Artifact, origin string) int {
	count := 0

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		got, err := measurement.Origin()
		if err == nil && got == origin {
			count++
		}
	}

	return count
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	if crypto.story != nil {
		if err := crypto.story.Close(); err != nil {
			return errnie.Error(err)
		}
	}

	if crypto.signals != nil {
		if err := crypto.signals.Close(); err != nil {
			return errnie.Error(err)
		}
	}

	if crypto.resonance != nil {
		if err := crypto.resonance.Close(); err != nil {
			return errnie.Error(err)
		}
	}

	if crypto.audit != nil {
		if err := crypto.audit.Close(); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}
