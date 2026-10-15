package hindsight

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/store"
)

type Run struct {
	Epoch     int64     `json:"epoch"`
	StartedAt time.Time `json:"started_at"`
	Status    string    `json:"status"`
}

type Service struct {
	ctx          context.Context
	client       *store.Client
	fragmentsMu  sync.RWMutex
	fragments    []TrainedFragmentResponse
	fragmentsAt  time.Time
	fragmentsMap map[int64]*data.Measurement
}

func NewService(ctx context.Context, client *store.Client) *Service {
	return &Service{
		ctx:          ctx,
		client:       client,
		fragmentsMap: make(map[int64]*data.Measurement),
	}
}

func (service *Service) Register(router fiber.Router) {
	if service == nil || router == nil {
		return
	}

	router.Get("/hindsight/metric-map", service.metricMap)
	router.Get("/hindsight/runs", service.runs)

	router.Use("/hindsight/timeline", func(ctx fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(ctx) {
			ctx.Locals("allowed", true)
			return ctx.Next()
		}

		return fiber.ErrUpgradeRequired
	})

	router.Get("/hindsight/timeline", websocket.New(service.timeline, websocket.Config{
		Origins: []string{"*"},
	}))

	router.Get("/hindsight/symbols", service.symbols)
	router.Get("/hindsight/excursions", service.excursions)
	router.Get("/hindsight/data", service.data)
	router.Get("/hindsight/captures", service.captures)
	router.Get("/hindsight/envelope", service.envelope)

	router.Get("/training/fragments", service.trainingFragments)
	router.Get("/training/fragments/:id/points", service.trainingFragmentPoints)

	router.Get("/hindsight/gaps", service.gaps)
	router.Get("/hindsight/lifecycle", service.lifecycle)
	router.Get("/hindsight/states", service.states)
	router.Get("/hindsight/state", service.state)
}

func (service *Service) metricMap(reqCtx fiber.Ctx) error {
	return reqCtx.JSON(signal.Semantics())
}

func (service *Service) runs(reqCtx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	runs := []Run{}
	seen := make(map[int64]bool)

	for page := range service.client.List("run/epoch/", "/") {
		for _, commonPrefix := range page.CommonPrefixes {
			if commonPrefix.Prefix == nil {
				continue
			}

			prefixStr := strings.TrimPrefix(*commonPrefix.Prefix, "run/epoch/")
			prefixStr = strings.Trim(prefixStr, "/")
			epoch, err := strconv.ParseInt(prefixStr, 10, 64)

			if err != nil || epoch <= 0 {
				continue
			}

			if seen[epoch] {
				continue
			}

			seen[epoch] = true
			runs = append(runs, Run{
				Epoch:     epoch,
				StartedAt: time.Unix(0, epoch).UTC(),
				Status:    "recorded",
			})
		}
	}

	return reqCtx.JSON(runs)
}

func (service *Service) timeline(conn *websocket.Conn) {
	if service.client == nil {
		return
	}

	run := conn.Query("run")

	if run == "" {
		run = conn.Query("epoch")
	}

	if run == "" {
		return
	}

	epoch := parseInt64Query(run)
	symbol := conn.Query("symbol")
	fromTick := parseInt64Query(conn.Query("from"))
	toTick := parseInt64Query(conn.Query("to"))

	const timelineBatchSize = 256
	batch := make([]*data.Measurement, 0, timelineBatchSize)

	prefix := "spot:trade/"

	if symbol != "" && epoch > 0 {
		prefix = fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)
	}

	for measurement := range service.client.Get(prefix) {
		if measurement == nil {
			continue
		}

		if epoch > 0 && measurement.Epoch != epoch {
			continue
		}

		if fromTick > 0 && measurement.Tick < fromTick {
			continue
		}

		if toTick > 0 && measurement.Tick > toTick {
			continue
		}

		batch = append(batch, measurement)

		if len(batch) < timelineBatchSize {
			continue
		}

		payload, err := data.EncodeMeasurements(batch)

		if err != nil {
			errnie.Error(err)
			return
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
			errnie.Error(err)
			return
		}

		batch = batch[:0]
	}

	if len(batch) > 0 {
		payload, err := data.EncodeMeasurements(batch)

		if err != nil {
			errnie.Error(err)
			return
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
			errnie.Error(err)
			return
		}
	}
}

func (service *Service) symbols(ctx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	symbols := []string{}
	seen := make(map[string]bool)

	for page := range service.client.List("spot:trade/", "/") {
		for _, commonPrefix := range page.CommonPrefixes {
			if commonPrefix.Prefix == nil {
				continue
			}

			symbol := strings.TrimPrefix(*commonPrefix.Prefix, "spot:trade/")
			symbol = strings.Trim(symbol, "/")

			if symbol != "" && !seen[symbol] {
				seen[symbol] = true
				symbols = append(symbols, symbol)
			}
		}
	}

	return ctx.JSON(symbols)
}

func (service *Service) excursions(ctx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := parseInt64Query(run)
	excursions := []*data.Measurement{}

	for measurement := range service.client.Get("detector/") {
		if measurement == nil {
			continue
		}

		if epoch > 0 && measurement.Epoch != epoch {
			continue
		}

		status := data.Pull(measurement.Meta("status"))

		if status != nil && status.Value == "resolved" {
			excursions = append(excursions, measurement)
		}
	}

	return ctx.JSON(excursions)
}

func (service *Service) data(ctx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	epoch := parseInt64Query(ctx.Query("epoch"))
	limit := int(parseUintQuery(ctx.Query("limit")))
	var measurements []*data.Measurement

	for measurement := range service.client.Get("spot:trade/") {
		if measurement == nil {
			continue
		}

		if epoch > 0 && measurement.Epoch != epoch {
			continue
		}

		measurements = append(measurements, measurement)

		if limit > 0 && len(measurements) >= limit {
			break
		}
	}

	if measurements == nil {
		measurements = []*data.Measurement{}
	}

	return ctx.JSON(measurements)
}

func (service *Service) captures(ctx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := parseInt64Query(run)
	after := parseInt64Query(ctx.Query("after"))
	symbol := ctx.Query("symbol")

	const maxCaptures = 64
	captures := make([]map[string]any, 0, maxCaptures)

	prefix := "spot:trade/"

	if symbol != "" && epoch > 0 {
		prefix = fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)
	}

	for measurement := range service.client.Get(prefix) {
		if measurement == nil {
			continue
		}

		if epoch > 0 && measurement.Epoch != epoch {
			continue
		}

		if measurement.SeqIdx < after {
			continue
		}

		receivedAt := ""

		if !measurement.At.IsZero() {
			receivedAt = measurement.At.UTC().Format("2006-01-02T15:04:05.000Z07:00")
		}

		captures = append(captures, map[string]any{
			"identity": map[string]any{
				"run":            strconv.FormatInt(epoch, 10),
				"sequence":       measurement.SeqIdx,
				"stream":         measurement.Source,
				"streamEpoch":    epoch,
				"streamSequence": measurement.SeqIdx,
			},
			"kind":       "ticker",
			"endpoint":   measurement.Source,
			"receivedAt": receivedAt,
		})

		if len(captures) >= maxCaptures {
			break
		}
	}

	return ctx.JSON(captures)
}

func (service *Service) envelope(ctx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := parseInt64Query(run)
	seq := parseInt64Query(ctx.Query("seq"))
	symbol := ctx.Query("symbol")

	if seq <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "seq required")
	}

	var found *data.Measurement

	prefix := "spot:trade/"

	if symbol != "" && epoch > 0 {
		prefix = fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)
	}

	for measurement := range service.client.Get(prefix) {
		if measurement == nil {
			continue
		}

		if epoch > 0 && measurement.Epoch != epoch {
			continue
		}

		if measurement.SeqIdx == seq && (symbol == "" || measurement.Label == symbol) {
			found = measurement
			break
		}
	}

	if found == nil {
		return fiber.NewError(fiber.StatusNotFound, "frame not found")
	}

	payloadBytes, err := json.Marshal(found)

	if err != nil {
		return readFailure(err)
	}

	receivedAt := ""

	if !found.At.IsZero() {
		receivedAt = found.At.UTC().Format("2006-01-02T15:04:05.000Z07:00")
	}

	capture := map[string]any{
		"identity": map[string]any{
			"run":            strconv.FormatInt(epoch, 10),
			"sequence":       found.SeqIdx,
			"stream":         found.Source,
			"streamEpoch":    epoch,
			"streamSequence": found.SeqIdx,
		},
		"kind":       "ticker",
		"endpoint":   found.Source,
		"receivedAt": receivedAt,
	}

	return ctx.JSON(map[string]any{
		"run":       strconv.FormatInt(epoch, 10),
		"sequence":  found.SeqIdx,
		"capture":   capture,
		"payload":   string(payloadBytes),
		"manifests": []any{},
		"witnesses": []any{},
	})
}

type HindsightGap struct {
	RunId    string `json:"runId"`
	Encoding string `json:"encoding"`
	Sequence int64  `json:"sequence"`
	Detail   string `json:"detail,omitempty"`
}

type HindsightLifecycleEvent struct {
	DecisionId string `json:"decisionId"`
	Symbol     string `json:"symbol"`
	Kind       string `json:"kind"`
	Action     string `json:"action"`
	At         string `json:"at"`
	CaptureSeq int64  `json:"captureSeq,omitempty"`
}

type HindsightCaptureIdentity struct {
	Run            string `json:"run"`
	Sequence       int64  `json:"sequence"`
	Stream         string `json:"stream"`
	StreamEpoch    int64  `json:"streamEpoch"`
	StreamSequence int64  `json:"streamSequence"`
}

type HindsightStateEnvelope struct {
	Origin  HindsightCaptureIdentity `json:"origin"`
	Ordinal int                      `json:"ordinal"`
}

type HindsightState struct {
	Envelope HindsightStateEnvelope `json:"envelope"`
	Payload  string                 `json:"payload"`
}

func (service *Service) gaps(ctx fiber.Ctx) error {
	if service.client == nil {
		return ctx.JSON([]HindsightGap{})
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := parseInt64Query(run)
	symbol := ctx.Query("symbol")
	gaps := make([]HindsightGap, 0)
	var prevSeq int64 = -1

	prefix := "spot:trade/"

	if symbol != "" && epoch > 0 {
		prefix = fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)
	}

	for measurement := range service.client.Get(prefix) {
		if measurement == nil {
			continue
		}

		if epoch > 0 && measurement.Epoch != epoch {
			continue
		}

		if prevSeq != -1 && measurement.SeqIdx > prevSeq+1 {
			gaps = append(gaps, HindsightGap{
				RunId:    strconv.FormatInt(measurement.Epoch, 10),
				Encoding: "sequence_gap",
				Sequence: prevSeq + 1,
				Detail:   fmt.Sprintf("Sequence jump from %d to %d (missing %d frames)", prevSeq, measurement.SeqIdx, measurement.SeqIdx-prevSeq-1),
			})
		}

		prevSeq = measurement.SeqIdx
	}

	return ctx.JSON(gaps)
}

func (service *Service) lifecycle(ctx fiber.Ctx) error {
	if service.client == nil {
		return ctx.JSON([]HindsightLifecycleEvent{})
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := parseInt64Query(run)
	symbol := ctx.Query("symbol")
	events := make([]HindsightLifecycleEvent, 0)

	// 1. Ingest detector excursions as lifecycle events
	detPrefix := "detector/"

	if symbol != "" && epoch > 0 {
		detPrefix = fmt.Sprintf("detector/%s/%019d/", symbol, epoch)
	}

	for measurement := range service.client.Get(detPrefix) {
		if measurement == nil || measurement.Source != "detector" {
			continue
		}

		if epoch > 0 && measurement.Epoch != epoch {
			continue
		}

		action := "excursion"

		if typeEntry := data.Pull(measurement.Meta("type")); typeEntry != nil && typeEntry.Value != "" {
			action = typeEntry.Value
		}

		atStr := ""

		if !measurement.At.IsZero() {
			atStr = measurement.At.UTC().Format("2006-01-02T15:04:05.000Z07:00")
		}

		events = append(events, HindsightLifecycleEvent{
			DecisionId: fmt.Sprintf("det-%d-%d", measurement.Epoch, measurement.SeqIdx),
			Symbol:     measurement.Label,
			Kind:       "detector",
			Action:     action,
			At:         atStr,
			CaptureSeq: measurement.SeqIdx,
		})
	}

	// 2. Ingest policy decisions as lifecycle events
	polPrefix := "policy/current/"

	if epoch > 0 {
		polPrefix = fmt.Sprintf("policy/current/%019d/", epoch)
	}

	keys := service.client.ListKeys(polPrefix)
	if len(keys) == 0 && symbol != "" && epoch > 0 {
		polPrefix = fmt.Sprintf("policy/%s/%019d/", symbol, epoch)
	}

	for measurement := range service.client.Get(polPrefix) {
		if measurement == nil || measurement.Source != "policy" {
			continue
		}

		if epoch > 0 && measurement.Epoch != epoch {
			continue
		}

		action := "noop"

		if actEntry := data.Pull(measurement.Meta("action")); actEntry != nil && actEntry.Value != "" {
			action = actEntry.Value
		}

		atStr := ""

		if !measurement.At.IsZero() {
			atStr = measurement.At.UTC().Format("2006-01-02T15:04:05.000Z07:00")
		}

		events = append(events, HindsightLifecycleEvent{
			DecisionId: fmt.Sprintf("pol-%d-%d", measurement.Epoch, measurement.SeqIdx),
			Symbol:     measurement.Label,
			Kind:       "policy",
			Action:     action,
			At:         atStr,
			CaptureSeq: measurement.SeqIdx,
		})
	}

	return ctx.JSON(events)
}

func (service *Service) states(ctx fiber.Ctx) error {
	if service.client == nil {
		return ctx.JSON([]HindsightState{})
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := parseInt64Query(run)
	symbol := ctx.Query("symbol")
	const maxStates = 64
	states := make([]HindsightState, 0, maxStates)

	prefix := "spot:trade/"

	if symbol != "" && epoch > 0 {
		prefix = fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)
	}

	for measurement := range service.client.Get(prefix) {
		if measurement == nil {
			continue
		}

		if epoch > 0 && measurement.Epoch != epoch {
			continue
		}

		payloadBytes, err := json.Marshal(measurement)

		if err != nil {
			continue
		}

		states = append(states, HindsightState{
			Envelope: HindsightStateEnvelope{
				Origin: HindsightCaptureIdentity{
					Run:            strconv.FormatInt(measurement.Epoch, 10),
					Sequence:       measurement.SeqIdx,
					Stream:         measurement.Source,
					StreamEpoch:    measurement.Epoch,
					StreamSequence: measurement.SeqIdx,
				},
				Ordinal: 0,
			},
			Payload: string(payloadBytes),
		})

		if len(states) >= maxStates {
			break
		}
	}

	return ctx.JSON(states)
}

func (service *Service) state(ctx fiber.Ctx) error {
	if service.client == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "capture store unavailable")
	}

	run := ctx.Query("run")

	if run == "" {
		run = ctx.Query("epoch")
	}

	epoch := parseInt64Query(run)
	seq := parseInt64Query(ctx.Query("seq"))
	ordinal := int(parseUintQuery(ctx.Query("ordinal")))
	symbol := ctx.Query("symbol")

	if seq <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "seq required")
	}

	var found *data.Measurement
	prefix := "spot:trade/"

	if symbol != "" && epoch > 0 {
		prefix = fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)
	}

	for measurement := range service.client.Get(prefix) {
		if measurement == nil {
			continue
		}

		if epoch > 0 && measurement.Epoch != epoch {
			continue
		}

		if measurement.SeqIdx == seq && (symbol == "" || measurement.Label == symbol) {
			found = measurement
			break
		}
	}

	if found == nil {
		return fiber.NewError(fiber.StatusNotFound, "no witnessed state at sequence")
	}

	payloadBytes, err := json.Marshal(found)

	if err != nil {
		return readFailure(err)
	}

	return ctx.JSON(HindsightState{
		Envelope: HindsightStateEnvelope{
			Origin: HindsightCaptureIdentity{
				Run:            strconv.FormatInt(found.Epoch, 10),
				Sequence:       found.SeqIdx,
				Stream:         found.Source,
				StreamEpoch:    found.Epoch,
				StreamSequence: found.SeqIdx,
			},
			Ordinal: ordinal,
		},
		Payload: string(payloadBytes),
	})
}

func readFailure(err error) error {
	return fiber.NewError(fiber.StatusInternalServerError, errnie.Error(err).Error())
}

func parseUintQuery(raw string) uint64 {
	value, err := strconv.ParseUint(raw, 10, 64)

	if err != nil {
		return 0
	}

	return value
}

func parseInt64Query(raw string) int64 {
	value, err := strconv.ParseInt(raw, 10, 64)

	if err != nil {
		return 0
	}

	return value
}

type FragmentPoint struct {
	X    int     `json:"x"`
	Y    float64 `json:"y"`
	Seq  int64   `json:"seq"`
	Time int64   `json:"time"`
}

type TrainedFragmentResponse struct {
	Id                int64           `json:"id"`
	Symbol            string          `json:"symbol"`
	Epoch             int64           `json:"epoch"`
	MarkA             int64           `json:"mark_a"`
	MarkB             int64           `json:"mark_b"`
	MarkC             int64           `json:"mark_c"`
	EntryPrice        float64         `json:"entry_price"`
	ExitPrice         float64         `json:"exit_price"`
	Magnitude         float64         `json:"magnitude"`
	Direction         string          `json:"direction"`
	Class             string          `json:"class,omitempty"`
	Tokens            []string        `json:"tokens"`
	Points            []FragmentPoint `json:"points"`
	EntryIdx          int64           `json:"entry_idx"`
	ExitIdx           int64           `json:"exit_idx"`
	PredictedEntryIdx int64           `json:"predicted_entry_idx"`
	PredictedExitIdx  int64           `json:"predicted_exit_idx"`
	LearnedAt         string          `json:"learned_at"`
}

func parseTick(measurement *data.Measurement, key string) int64 {
	if entry := data.Pull(measurement.Meta(key)); entry != nil {
		val, err := strconv.ParseInt(entry.Value, 10, 64)

		if err == nil {
			return val
		}
	}

	if entry := data.Pull(measurement.Read(key)); entry != nil && entry.Metric != nil {
		return int64(entry.Metric.Raw())
	}

	return 0
}

func parsePrice(measurement *data.Measurement, key string) float64 {
	if entry := data.Pull(measurement.Meta(key)); entry != nil {
		val, err := strconv.ParseFloat(entry.Value, 64)

		if err == nil {
			return val
		}
	}

	if entry := data.Pull(measurement.Read(key)); entry != nil && entry.Metric != nil {
		return entry.Metric.Raw()
	}

	return 0
}

func filterFragmentsByEpoch(fragments []TrainedFragmentResponse, epoch int64) []TrainedFragmentResponse {
	if epoch <= 0 {
		return fragments
	}

	filtered := make([]TrainedFragmentResponse, 0)
	for _, f := range fragments {
		if f.Epoch == epoch {
			filtered = append(filtered, f)
		}
	}

	return filtered
}

func (service *Service) getFragments(epoch int64) ([]TrainedFragmentResponse, error) {
	if service.client == nil {
		return []TrainedFragmentResponse{}, nil
	}

	service.fragmentsMu.RLock()
	if len(service.fragments) > 0 && time.Since(service.fragmentsAt) < 30*time.Second {
		cached := service.fragments
		service.fragmentsMu.RUnlock()
		return filterFragmentsByEpoch(cached, epoch), nil
	}
	service.fragmentsMu.RUnlock()

	service.fragmentsMu.Lock()
	defer service.fragmentsMu.Unlock()

	if len(service.fragments) > 0 && time.Since(service.fragmentsAt) < 30*time.Second {
		return filterFragmentsByEpoch(service.fragments, epoch), nil
	}

	fragments := make([]TrainedFragmentResponse, 0)
	fragmentsMap := make(map[int64]*data.Measurement)
	var autoID int64 = 1

	for det := range service.client.Get("detector/") {
		if det == nil || det.Source != "detector" {
			continue
		}

		id := autoID
		autoID++

		if det.ID > 0 {
			id = int64(det.ID)
		}

		fragmentsMap[id] = det

		startTick := parseTick(det, "start_tick")
		bTick := parseTick(det, "b_tick")
		cTick := parseTick(det, "c_tick")
		entryPrice := parsePrice(det, "b_price")
		exitPrice := parsePrice(det, "c_price")

		var magnitude float64

		if entryPrice > 0 {
			magnitude = (exitPrice - entryPrice) / entryPrice
		}

		direction := "up"

		if exitPrice < entryPrice {
			direction = "down"
		}

		class := direction

		if entry := data.Pull(det.Meta("type")); entry != nil && entry.Value != "" {
			class = entry.Value
		}

		learnedAt := ""

		if !det.At.IsZero() {
			learnedAt = det.At.UTC().Format("2006-01-02T15:04:05Z07:00")
		}

		var startIdx int64
		var bIdx int64
		var cIdx int64

		if entry := data.Pull(det.Read("start_idx")); entry != nil && entry.Metric != nil {
			startIdx = int64(entry.Metric.Raw())
		}

		if entry := data.Pull(det.Read("b_idx")); entry != nil && entry.Metric != nil {
			bIdx = int64(entry.Metric.Raw())
		}

		if entry := data.Pull(det.Read("c_idx")); entry != nil && entry.Metric != nil {
			cIdx = int64(entry.Metric.Raw())
		}

		entryIdx := max(bIdx-startIdx, 0)
		exitIdx := max(cIdx-startIdx, entryIdx)

		var predictedEntryIdx int64 = -1
		var predictedExitIdx int64 = -1

		if pEntry := data.Pull(det.Read("predicted_entry")); pEntry != nil && pEntry.Metric != nil && pEntry.Metric.Raw() >= 0 {
			predictedEntryIdx = int64(pEntry.Metric.Raw())
		}

		if pExit := data.Pull(det.Read("predicted_exit")); pExit != nil && pExit.Metric != nil && pExit.Metric.Raw() >= 0 {
			predictedExitIdx = int64(pExit.Metric.Raw())
		}

		fragments = append(fragments, TrainedFragmentResponse{
			Id:                id,
			Symbol:            det.Label,
			Epoch:             det.Epoch,
			MarkA:             startTick,
			MarkB:             bTick,
			MarkC:             cTick,
			EntryPrice:        entryPrice,
			ExitPrice:         exitPrice,
			Magnitude:         magnitude,
			Direction:         direction,
			Class:             class,
			Tokens:            []string{},
			Points:            []FragmentPoint{},
			EntryIdx:          entryIdx,
			ExitIdx:           exitIdx,
			PredictedEntryIdx: predictedEntryIdx,
			PredictedExitIdx:  predictedExitIdx,
			LearnedAt:         learnedAt,
		})
	}

	service.fragments = fragments
	service.fragmentsMap = fragmentsMap
	service.fragmentsAt = time.Now()

	return filterFragmentsByEpoch(fragments, epoch), nil
}

func (service *Service) trainingFragments(ctx fiber.Ctx) error {
	if service.client == nil {
		return ctx.JSON([]TrainedFragmentResponse{})
	}

	epoch := parseInt64Query(ctx.Query("epoch"))
	fragments, err := service.getFragments(epoch)

	if err != nil {
		return err
	}

	return ctx.JSON(fragments)
}

func fetchFragmentPoints(client *store.Client, symbol string, epoch, startTick, cTick int64) []FragmentPoint {
	if client == nil {
		return []FragmentPoint{}
	}

	points := make([]FragmentPoint, 0)
	var x int
	pointsPrefix := fmt.Sprintf("spot:trade/%s/%019d/", symbol, epoch)

	for m := range client.Get(pointsPrefix) {
		if m == nil {
			continue
		}

		if startTick > 0 && m.Tick < startTick {
			continue
		}

		if cTick > 0 && m.Tick > cTick {
			break
		}

		var price float64

		if pEntry := data.Pull(m.Read("price")); pEntry != nil && pEntry.Metric != nil {
			price = pEntry.Metric.Raw()
		}

		if price <= 0 {
			continue
		}

		var timeMs int64

		if !m.At.IsZero() {
			timeMs = m.At.UnixMilli()
		}

		points = append(points, FragmentPoint{
			X:    x,
			Y:    price,
			Seq:  m.Tick,
			Time: timeMs,
		})

		x++
	}

	return points
}

func (service *Service) trainingFragmentPoints(ctx fiber.Ctx) error {
	if service.client == nil {
		return ctx.JSON([]FragmentPoint{})
	}

	targetID := parseInt64Query(ctx.Params("id"))

	service.fragmentsMu.RLock()
	targetDet := service.fragmentsMap[targetID]
	service.fragmentsMu.RUnlock()

	if targetDet == nil {
		_, _ = service.getFragments(0)
		service.fragmentsMu.RLock()
		targetDet = service.fragmentsMap[targetID]
		service.fragmentsMu.RUnlock()
	}

	if targetDet == nil {
		return ctx.JSON([]FragmentPoint{})
	}

	startTick := parseTick(targetDet, "start_tick")
	cTick := parseTick(targetDet, "c_tick")

	if startTick <= 0 && targetDet.Tick > 0 {
		startTick = targetDet.Tick
	}

	if cTick <= 0 && targetDet.SeqIdx > 0 {
		cTick = targetDet.SeqIdx
	}

	points := fetchFragmentPoints(service.client, targetDet.Label, targetDet.Epoch, startTick, cTick)
	return ctx.JSON(points)
}
