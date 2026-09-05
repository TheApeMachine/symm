package strategy

import (
	"github.com/theapemachine/symm/nomagique/learning"
	"time"
)

/* LearningView is an on-demand, immutable copy for the operator dashboard. */
type LearningView struct {
	Capital        CapitalView       `json:"capital"`
	Warmup         WarmupReading     `json:"warmup"`
	At             time.Time         `json:"at"`
	Symbol         string            `json:"symbol"`
	Status         string            `json:"status"`
	Steps          uint64            `json:"steps"`
	Decisions      uint64            `json:"decisions"`
	Resolved       uint64            `json:"resolved"`
	GridVersion    uint64            `json:"gridVersion"`
	Columns        int               `json:"columns"`
	InitialCapital string            `json:"initialCapital"`
	Universe       []LearningSummary `json:"universe"`
	Regions        []learning.Region `json:"regions"`
	Points         []LearningPoint   `json:"points"`
	Lanes          []LearningWallet  `json:"lanes"`

	/*
		Skill is the measured competence of the policy lane and the execution
		authority it justifies. Dispatched counts intents actually handed to an
		account: it is zero in a learning-only run by construction.
	*/
	Skill              SkillReading `json:"skill"`
	AuthorizedMode     string       `json:"authorizedMode"`
	RealizationAllowed bool         `json:"realizationAllowed"`
	RealizationReason  string       `json:"realizationReason,omitempty"`
	Dispatched         uint64       `json:"dispatched"`

	/*
		Rejected counts policy intents the account did not accept, with the
		most recent reason. The agent decides from its own simulated wallet, so
		its intent and the account's actual position can disagree; that
		disagreement is reported here rather than stopping the run.
	*/
	Rejected  uint64 `json:"rejected"`
	Rejection string `json:"rejection,omitempty"`

	/*
		Execution is the account's own account of what it did with those
		intents. It is absent when the attached desk cannot report one, rather
		than filled in with zeros that would read as "nothing went wrong".
	*/
	Execution    ExecutionStatus `json:"execution"`
	HasExecution bool            `json:"hasExecution"`

	/*
		Forward is what the tape actually offered, reviewed behind real time,
		against what the policy lane was holding while it happened.
	*/
	Forward ForwardReview `json:"forward"`

	/*
		Horizon is the forward window every decision in this market is scored
		over, derived from Epochs observed impulse changes averaging EpochMean
		seconds. Until an interval has been observed the horizon is zero and
		nothing resolves.
	*/
	Horizon       time.Duration `json:"horizonNs"`
	HorizonEpochs int           `json:"horizonEpochs"`
	EpochMean     float64       `json:"epochMean"`
	Epochs        uint64        `json:"epochs"`

	/*
		Impulse is the ordered context the next decision is conditioned on,
		with each token resolved back to the quantity it names. Candidates are
		the feasible actions at that context with the evidence recalled for
		each, so a chosen action can be read against the ones it beat.
	*/
	Impulse    []LearningToken     `json:"impulse"`
	Candidates []LearningCandidate `json:"candidates"`

	/*
		Influence ranks which measured quantities have accumulated outcome
		evidence for which actions. It is association under the agent's own
		exploration, not a controlled comparison.
	*/
	Influence []MetricInfluence `json:"influence"`
}

/* LearningToken resolves one context token back to the quantity it identifies. */
type LearningToken struct {
	Token     uint64  `json:"token"`
	Source    string  `json:"source"`
	Label     string  `json:"label"`
	Strength  float64 `json:"strength"`
	Authority float64 `json:"authority"`
	Members   int     `json:"members"`
}

/*
LearningCandidate is one feasible action at the current context together with
the evidence recalled for it. Selected marks the action the policy lane would
take now; an undefined prior means this action has never completed here, which
is why exploration would reach for it.
*/
type LearningCandidate struct {
	Knowledge KnowledgeReading      `json:"knowledge"`
	Kind      string                `json:"kind"`
	Power     uint16                `json:"power"`
	Reduce    bool                  `json:"reduce"`
	Selected  bool                  `json:"selected"`
	Prior     learning.PriorReading `json:"prior"`
}

/* LearningSummary locates active independent contexts without combining capital. */
type LearningSummary struct {
	Symbol    string `json:"symbol"`
	Status    string `json:"status"`
	Decisions uint64 `json:"decisions"`
}

/* LearningPoint exposes a cell's position and quality-conditioned activity. */
type LearningPoint struct {
	ID        uint64  `json:"id"`
	Source    string  `json:"source"`
	Label     string  `json:"label"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Value     float64 `json:"value"`
	Energy    float64 `json:"energy"`
	Authority float64 `json:"authority"`
	Present   bool    `json:"present"`
}

/* LearningWallet keeps each cloned account's actual economics separate. */
type LearningWallet struct {
	Lane       int                   `json:"lane"`
	Mode       string                `json:"mode"`
	Cash       string                `json:"cash"`
	Quantity   string                `json:"quantity"`
	Fees       string                `json:"fees"`
	Equity     float64               `json:"equity"`
	Profit     float64               `json:"profit"`
	Rate       float64               `json:"rate"`
	At         time.Time             `json:"at"`
	Complete   bool                  `json:"complete"`
	Action     LearningAction        `json:"action"`
	Pending    bool                  `json:"pending"`
	Issued     uint64                `json:"issued"`
	Fills      uint64                `json:"fills"`
	Resolved   uint64                `json:"resolved"`
	Unresolved int                   `json:"unresolved"`
	Prior      learning.PriorReading `json:"prior"`

	/*
		Episodes counts finished accounts: a lane that spent its capital on
		execution costs restarts on a fresh clone of the same known balance.
		Realized is what those finished episodes actually returned and Spent is
		what they paid in fees. Neither is a balance anyone holds, and they are
		never summed across lanes.
	*/
	Episodes  uint64  `json:"episodes"`
	Realized  float64 `json:"realized"`
	Spent     float64 `json:"spent"`
	Exhausted bool    `json:"exhausted"`
}
