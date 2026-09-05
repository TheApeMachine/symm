import type { MetricSemantics } from "./hindsight-types";

/*
Reading — the plain-language layer over declared metric semantics.

Everything a reader is told here is something the backend already stated. The
METRIC_MAP declares a metric's role, class, purpose, definedness and forbidden
use in the vocabulary of the system; this file translates that vocabulary into
the words a non-specialist reads, and nothing else. It never inspects a value
to decide whether the number is good, high, healthy, or tradeable — no such
statement exists in the map, so the surface has none to make.

Two claims are load-bearing:

  1. A translation is of the declared term, never of the measurement. Role
     ARRIVAL_MODEL_STATE always reads the same way regardless of what the
     number was, because the declaration is about the metric, not the sample.

  2. An undeclared term is passed through rather than guessed at. A role this
     file has never seen is shown as the system spells it, which is honest and
     obviously incomplete, instead of being folded into a nearby meaning.
*/

/*
RoleReading restates a declared semantic role: what family of question the
metric answers, and the everyday phrasing of it.
*/
export type RoleReading = {
	title: string;
	plain: string;
};

const ROLES: Record<string, RoleReading> = {
	ARRIVAL_MODEL_STATE: {
		title: "Event timing",
		plain:
			"How trades are arriving in time — the rhythm and clustering of activity, not its direction.",
	},
	FLOW_STATE: {
		title: "Executed flow",
		plain:
			"What actually traded and which side was the aggressor — orders that hit, not orders on display.",
	},
	LIQUIDITY_DISPOSITION: {
		title: "Available liquidity",
		plain:
			"What size is displayed to trade against and what it costs to cross — the shape of the book right now.",
	},
	ACTIVITY_STATE: {
		title: "Activity level",
		plain:
			"How much is happening on this instrument compared with its own normal.",
	},
	MUTATION_STATE: {
		title: "Book changes",
		plain:
			"How displayed orders are being added, pulled, and moved between observations.",
	},
	RELATIONSHIP_STATE: {
		title: "Relationship to other instruments",
		plain:
			"How this instrument's moves line up with another's. Alignment only — never who caused what.",
	},
	CROSS_SECTIONAL_CONTEXT: {
		title: "The wider market",
		plain:
			"Where this instrument sits relative to the cohort around it, rather than to its own history.",
	},
	DERIVATIVE_CONTEXT: {
		title: "Derivative context",
		plain:
			"Facts specific to a derivative instrument and how it sits against its reference price.",
	},
	EXECUTION_CONTEXT: {
		title: "Cost of trading",
		plain:
			"What a trade would run into on the way through — the frictions around getting filled.",
	},
	ESTIMABILITY: {
		title: "Can this be known?",
		plain:
			"How well-supported the estimate is. This qualifies other numbers; it is never a signal on its own.",
	},
	CONTEXT_OR_MODEL_FEATURE: {
		title: "Model context",
		plain:
			"A supporting quantity that other calculations read. It qualifies a picture rather than making one.",
	},
	CONTEXT_INPUT: {
		title: "Model input",
		plain: "A declared input another calculation consumes.",
	},
	DEPRECATE_REDUNDANT: {
		title: "Superseded",
		plain:
			"Kept for compatibility and duplicated by a canonical source. Do not build on it.",
	},
};

/*
ClassReading restates a declared metric class: what kind of quantity the number
is, which is what tells a reader how to compare two of them.
*/
export type ClassReading = {
	title: string;
	plain: string;
};

const CLASSES: Record<string, ClassReading> = {
	direct_or_derived_measurement: {
		title: "Measured quantity",
		plain: "A quantity read from the market record, in its own real units.",
	},
	derived_measurement: {
		title: "Measured quantity",
		plain: "Calculated from other measured quantities, in real units.",
	},
	derived_dimensionless: {
		title: "Ratio",
		plain:
			"A ratio or share with no units, so it compares across instruments of any price.",
	},
	temporal_dynamic_or_rate: {
		title: "Rate of change",
		plain: "How fast something is moving, rather than where it stands.",
	},
	fitted_model_quantity: {
		title: "Model estimate",
		plain:
			"Fitted by a model rather than read off directly, so it carries the model's uncertainty.",
	},
	standardized_historical_comparison: {
		title: "Versus its own history",
		plain:
			"Distance from this instrument's own baseline, in standard deviations. Roughly: 0 is normal, ±2 is unusual.",
	},
	historical_comparison: {
		title: "Versus its own history",
		plain: "Compared against this instrument's own past behaviour.",
	},
	historical_reference: {
		title: "Baseline",
		plain:
			"The reference level the comparisons above are measured against — the 'normal' itself.",
	},
	support_or_inference: {
		title: "Confidence support",
		plain:
			"Describes how much the estimate can be relied on, not what the market did.",
	},
	structural_metric: {
		title: "Structure",
		plain: "A structural property of the book rather than a market movement.",
	},
};

/* readRole restates a declared role, or passes an undeclared one through. */
export const readRole = (role?: string): RoleReading | null => {
	if (!role) return null;

	return ROLES[role] ?? { title: role, plain: "" };
};

/* readClass restates a declared class, or passes an undeclared one through. */
export const readClass = (metricClass?: string): ClassReading | null => {
	if (!metricClass) return null;

	return CLASSES[metricClass] ?? { title: metricClass, plain: "" };
};

/*
Support is the declared trustworthiness of one measurement, as the estimator
itself reported it. Maturity is the estimator's own statement of how much
support stands behind the number, and SNR its statement of how far the number
stands above its own noise.

The bands are the estimator's scale read back, not a judgement added on top:
maturity is a 0..1 support fraction, and an SNR at or below 1 is a quantity no
larger than the noise around it. Nothing here says the market is good or bad.
*/
export type Support = {
	tone: "strong" | "fair" | "weak";
	label: string;
	plain: string;
};

export const readSupport = (maturity: number, snr: number | null): Support => {
	const noisy = snr !== null && snr <= 1;

	if (maturity >= 0.8 && !noisy) {
		return {
			tone: "strong",
			label: "well supported",
			plain:
				"The estimator reports near-full support for this reading, and where it defines a signal-to-noise it is above its own noise.",
		};
	}

	if (maturity >= 0.4 && !noisy) {
		return {
			tone: "fair",
			label: "partly supported",
			plain:
				"The estimator has some support behind this reading but not its full span. Treat it as provisional.",
		};
	}

	if (noisy) {
		return {
			tone: "weak",
			label: "at or below noise",
			plain:
				"The estimator reports this quantity as no larger than the noise around it. It is not evidence of anything on its own.",
		};
	}

	return {
		tone: "weak",
		label: "thin support",
		plain:
			"The estimator reports little support behind this reading — it has not yet seen enough to stand behind it.",
	};
};

/*
StanceReading restates how a category hypothesis used this metric as evidence
at this exact boundary. This is the one place the surface can say a number
mattered, because the running system said so itself.
*/
export const readStance = (
	stance: string,
): { label: string; plain: string; tone: "up" | "down" | "muted" } => {
	if (stance === "supports") {
		return {
			label: "argued for",
			plain:
				"This reading was counted as evidence for that reading of the market.",
			tone: "up",
		};
	}

	if (stance === "contradicts") {
		return {
			label: "argued against",
			plain:
				"This reading was counted as evidence against that reading of the market.",
			tone: "down",
		};
	}

	return {
		label: "wanted, not available",
		plain:
			"That reading of the market wanted this evidence and could not get it here.",
		tone: "muted",
	};
};

/*
undeclared is the sentence shown where METRIC_MAP has no entry. It is stated
rather than softened: a number the system has not declared a meaning for is
exactly the kind of thing a reader must not quietly build a conclusion on.
*/
export const UNDECLARED =
	"The system has not declared what this number means, so this surface will not tell you. Treat it as unexplained.";

/*
summarise builds the one-line answer to "what am I looking at?" for a metric,
from the declared purpose where there is one.
*/
export const summarise = (declared: MetricSemantics | null): string =>
	declared?.purpose?.trim() || UNDECLARED;
