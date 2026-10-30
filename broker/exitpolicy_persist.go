package broker

import (
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
)

const (
	exitPolicyObjectDir = "model/parameters/exitpolicy/"
	exitShadowQueueKey  = "model/parameters/exitpolicy/shadow_queue.bin"
)

type persistedExitPolicy struct {
	Episodes []persistedEpisode   `json:"episodes"`
	Policy   persistedLearnPolicy `json:"policy"`
}

type persistedEpisode struct {
	Trace          []float64 `json:"trace"`
	Banked         float64   `json:"banked"`
	DurationNs     int64     `json:"duration_ns"`
	TimeToPeakNs   int64     `json:"time_to_peak_ns"`
	PostExitMaxPct float64   `json:"post_exit_max_pct,omitempty"`
	RegretPct      float64   `json:"regret_pct,omitempty"`
	ShadowBars     int       `json:"shadow_bars,omitempty"`
}

type persistedLearnPolicy struct {
	InvalidationCut float64 `json:"invalidation_cut"`
	TrailingStop    float64 `json:"trailing_stop"`
	MaxHoldNs       int64   `json:"max_hold_ns"`
	MaxHoldBars     int     `json:"max_hold_bars"`
	Worst           float64 `json:"worst"`
	Samples         int     `json:"samples"`
}

func exitPolicyObjectKey(symbol string) string {
	return exitPolicyObjectDir + strings.ReplaceAll(symbol, "/", "-") + ".bin"
}

func symbolFromExitPolicyKey(key string) string {
	key = strings.TrimPrefix(key, exitPolicyObjectDir)
	key = strings.TrimSuffix(key, ".bin")
	if key == "" || key == "shadow_queue" {
		return ""
	}

	return strings.ReplaceAll(key, "-", "/")
}

func (policy *ExitPolicy) Restore() {
	desk := policy.desk
	if policy == nil || desk == nil || desk.history == nil || desk.history.store == nil {
		return
	}

	ctx := desk.Context()
	for key := range desk.history.store.ListKeys(exitPolicyObjectDir) {
		symbol := symbolFromExitPolicyKey(key)
		if symbol == "" {
			continue
		}

		value := desk.history.store.Download(ctx, key)
		if value == nil || value.Err != nil {
			continue
		}

		record := decodePolicyRecord(value)
		if record == nil {
			continue
		}

		policy.policies.Store(symbol, record)
	}

	policy.restoreShadowQueue()
}

func decodePolicyRecord(value *data.Value) *policyRecord {
	if value == nil {
		return nil
	}

	var blob persistedExitPolicy
	value.Lookup(&blob, "exit_policy")

	if len(blob.Episodes) == 0 && blob.Policy.Samples == 0 {
		return nil
	}

	record := &policyRecord{
		episodes: make([]episode, 0, len(blob.Episodes)),
		policy: learnedExitPolicy{
			invalidationCut: blob.Policy.InvalidationCut,
			trailingStop:    blob.Policy.TrailingStop,
			maxHoldDuration: time.Duration(blob.Policy.MaxHoldNs),
			maxHoldBars:     blob.Policy.MaxHoldBars,
			worst:           blob.Policy.Worst,
			samples:         blob.Policy.Samples,
		},
	}

	for _, pe := range blob.Episodes {
		record.episodes = append(record.episodes, episode{
			trace:          append([]float64(nil), pe.Trace...),
			banked:         pe.Banked,
			duration:       time.Duration(pe.DurationNs),
			timeToPeak:     time.Duration(pe.TimeToPeakNs),
			postExitMaxPct: pe.PostExitMaxPct,
			regretPct:      pe.RegretPct,
			shadowBars:     pe.ShadowBars,
		})
	}

	record.rows = policyRows(record.episodes)
	return record
}

func (policy *ExitPolicy) persist(symbol string) {
	desk := policy.desk
	if policy == nil || desk == nil || desk.history == nil || desk.history.store == nil || symbol == "" {
		return
	}

	known := policy.record(symbol)
	if known == nil {
		return
	}

	known.mu.Lock()
	blob := encodePolicyRecord(known)
	known.mu.Unlock()

	value := data.NewValue("model", "parameters", "exitpolicy")
	value.Append(blob, "exit_policy")

	if _, err := desk.history.store.WriteAtPrefix(exitPolicyObjectKey(symbol), value.Bytes()); err != nil {
		errnie.Error(errnie.Err(errnie.IO, "[exitpolicy] persist "+symbol, err))
	}
}

func encodePolicyRecord(known *policyRecord) persistedExitPolicy {
	if known == nil {
		return persistedExitPolicy{}
	}

	out := persistedExitPolicy{
		Episodes: make([]persistedEpisode, 0, len(known.episodes)),
		Policy: persistedLearnPolicy{
			InvalidationCut: known.policy.invalidationCut,
			TrailingStop:    known.policy.trailingStop,
			MaxHoldNs:       known.policy.maxHoldDuration.Nanoseconds(),
			MaxHoldBars:     known.policy.maxHoldBars,
			Worst:           known.policy.worst,
			Samples:         known.policy.samples,
		},
	}

	for _, ep := range known.episodes {
		out.Episodes = append(out.Episodes, persistedEpisode{
			Trace:          append([]float64(nil), ep.trace...),
			Banked:         ep.banked,
			DurationNs:     ep.duration.Nanoseconds(),
			TimeToPeakNs:   ep.timeToPeak.Nanoseconds(),
			PostExitMaxPct: ep.postExitMaxPct,
			RegretPct:      ep.regretPct,
			ShadowBars:     ep.shadowBars,
		})
	}

	return out
}

func (policy *ExitPolicy) persistShadowQueue() {
	desk := policy.desk
	if policy == nil || desk == nil || desk.history == nil || desk.history.store == nil {
		return
	}

	policy.shadowMu.Lock()
	jobs := append([]shadowJob(nil), policy.shadowJobs...)
	policy.shadowMu.Unlock()

	raw, err := sonic.Marshal(jobs)
	if err != nil {
		errnie.Error(errnie.Err(errnie.IO, "[desk] marshal shadow queue", err))
		return
	}

	value := data.NewValue("model", "parameters", "exitpolicy")
	value.AppendBytes(raw, "shadow_queue")

	if _, err := desk.history.store.WriteAtPrefix(exitShadowQueueKey, value.Bytes()); err != nil {
		errnie.Error(errnie.Err(errnie.IO, "[exitpolicy] persist shadow queue", err))
	}
}

func (policy *ExitPolicy) restoreShadowQueue() {
	desk := policy.desk
	if policy == nil || desk == nil || desk.history == nil || desk.history.store == nil {
		return
	}

	value := desk.history.store.Download(desk.Context(), exitShadowQueueKey)
	if value == nil || value.Err != nil {
		return
	}

	var jobs []shadowJob
	value.Lookup(&jobs, "shadow_queue")

	if len(jobs) == 0 {
		return
	}

	policy.shadowMu.Lock()
	policy.shadowJobs = jobs
	policy.shadowMu.Unlock()
}
