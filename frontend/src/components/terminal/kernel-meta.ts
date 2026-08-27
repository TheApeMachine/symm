import type { Variant } from "@/components/ui/types";

export type SignalHealthStatus =
	| "waiting"
	| "standby"
	| "unfocused"
	| "calibrating"
	| "fault"
	| "ambiguous"
	| "measured"
	| "unknown";

export type KernelStatusMeta = {
	label: string;
	fg: string;
	bg: string;
	bd: string;
};

export const TERMINAL_KERNEL_ORDER = [
	"prediction",
	"resonance",
	"hawkes",
	"cognitive",
	"causal",
	"manifold",
	"regime",
	"correlation",
	"derivatives",
	"pumpdump",
	"toxicity",
	"exhaustion",
	"cvd",
	"depthflow",
	"liquidity",
	"sentiment",
	"leadlag",
] as const;

const kernelOrderIndex = (source: string): number => {
	const index = TERMINAL_KERNEL_ORDER.indexOf(
		source as (typeof TERMINAL_KERNEL_ORDER)[number],
	);

	return index === -1 ? TERMINAL_KERNEL_ORDER.length : index;
};

/*
Each kernel publishes its own vocabulary, so the headline is named per source
rather than assumed. The binding must name a metric that the live producer
actually emits; otherwise the row can be live while its trace remains empty.

Headlines are the actual emitted metric names, verified against the signal
bindings: signed_correlation (correlation), signed_net_fraction_zscore (cvd),
book_imbalance (depthflow), open_interest_growth_zscore (derivatives),
book_imbalance_zscore (exhaustion), branching_spectral_radius (hawkes),
best_lag_correlation (leadlag), touch_notional_imbalance (liquidity),
spread_zscore (pumpdump), breadth (sentiment), fill_fraction_zscore:bid
(toxicity).
*/
const SOURCE_HEADLINE: Record<string, string> = {
	correlation: "signed_correlation",
	cvd: "signed_net_fraction_zscore",
	depthflow: "book_imbalance",
	derivatives: "open_interest_growth_zscore",
	exhaustion: "book_imbalance_zscore",
	hawkes: "branching_spectral_radius",
	leadlag: "best_lag_correlation",
	liquidity: "touch_notional_imbalance",
	pumpdump: "spread_zscore",
	sentiment: "breadth",
	toxicity: "fill_fraction_zscore:bid",
};

/*
sourceHeadline names the metric a kernel row leads with, or null when the
source has no named headline. The list view prefers it so a kernel's trace
plots its own vocabulary instead of the first metric in the row.
*/
export const sourceHeadline = (source: string): string | null =>
	SOURCE_HEADLINE[source.toLowerCase()] ?? null;

/*
sourceHeadlineMetric names the metric map entry a kernel row leads with.
*/
export const sourceHeadlineMetric = (source: string): string => {
	const metric = sourceHeadline(source);

	if (metric === null) {
		throw new Error(`unsupported measurement source: ${source}`);
	}

	return `metrics.${metric}`;
};

/*
A kernel publishes a map of named metrics, and the detail panel reads several of
them side by side. The map cannot be walked from a binding — a painted surface
names the paths it wants up front — so each kernel's readable metrics are named
here, in the vocabulary the signal actually emits.

A name that a kernel turns out not to publish reads as absent rather than as a
zero, so the panel understates rather than invents.
*/
const SOURCE_METRICS: Record<string, string[]> = {
	correlation: [
		"last_price",
		"signed_correlation",
		"absolute_correlation",
		"cohort_signed_correlation",
		"correlation_zscore",
		"correlation_p_value",
		"overlap_density",
		"effective_sample_count",
	],
	cvd: [
		"trade_count",
		"signed_count_fraction",
		"executed_quantity:buy",
		"executed_quantity:sell",
		"gross_notional",
		"net_notional",
		"signed_net_fraction",
		"signed_net_fraction_zscore",
		"gross_notional_rate",
		"gross_notional_rate_zscore",
		"cumulative_volume_delta",
		"midpoint_log_return",
	],
	depthflow: [
		"book_imbalance",
		"touch_imbalance",
		"book_imbalance_zscore",
		"imbalance_resolution_gap",
		"flow_activity_imbalance",
		"net_displayed_flow:bid",
		"net_displayed_flow:ask",
		"signed_net_displayed_flow_rate",
	],
	derivatives: [
		"derivative_price",
		"reference_price",
		"basis",
		"basis_zscore",
		"open_interest",
		"open_interest_growth_zscore",
		"return_gap_zscore",
		"open_interest_growth_baseline",
	],
	exhaustion: [
		"displayed_depth_notional",
		"spread",
		"relative_spread",
		"book_imbalance",
		"book_imbalance_zscore",
		"depth_zscore:bid",
		"depth_zscore:ask",
		"midpoint_log_return",
	],
	hawkes: [
		"arrival_rate",
		"arrival_rate:buy",
		"arrival_rate:sell",
		"conditional_intensity",
		"conditional_intensity:buy",
		"conditional_intensity:sell",
		"background_rate",
		"branching_spectral_radius",
		"event_count",
		"event_fraction:buy",
	],
	leadlag: [
		"contemporaneous_correlation",
		"best_lag_correlation",
		"best_lag_correlation_zscore",
		"best_lag_index",
		"best_lag_seconds",
		"lag_fraction",
		"absolute_correlation_gain",
		"effective_sample_count",
	],
	liquidity: [
		"best_bid_price",
		"best_ask_price",
		"midpoint",
		"spread",
		"relative_spread",
		"touch_quantity:bid",
		"touch_quantity:ask",
		"touch_notional_imbalance",
		"two_sided_touch_notional",
	],
	pumpdump: [
		"best_bid",
		"best_ask",
		"midpoint",
		"spread",
		"relative_spread",
		"spread_ratio",
		"spread_divergence",
		"spread_zscore",
		"trade_price",
		"trade_quantity",
		"notional_rate",
		"notional_rate_zscore",
	],
	sentiment: [
		"advance_count",
		"decline_count",
		"advance_fraction",
		"breadth",
		"breadth_zscore",
		"median_return",
		"largest_absolute_return",
		"directional_agreement",
	],
	toxicity: [
		"touch_fill_fraction:bid",
		"touch_fill_fraction:ask",
		"fill_fraction_baseline:bid",
		"fill_fraction_divergence:bid",
		"fill_fraction_zscore:bid",
		"fill_fraction_zscore:ask",
		"net_withdrawal_fraction:bid",
		"withdrawal_fraction_zscore:bid",
	],
};

/*
sourceMetrics names the metrics a kernel's detail panel reads. An unsupported
source is a wiring error; silently painting a generic metric would make that
frontend defect indistinguishable from missing backend data. "snr" is prepended
first: every measurement row serializes its signal-to-noise ratio under that
name, so it is the one quantity every kernel's detail always shows.
*/
export const sourceMetrics = (source: string): string[] => {
	const metrics = SOURCE_METRICS[source.toLowerCase()];

	if (metrics === undefined) {
		throw new Error(`unsupported measurement source: ${source}`);
	}

	return metrics.includes("snr") ? metrics : ["snr", ...metrics];
};

/*
metricLabel reads a wire metric name — snake cased, optionally suffixed with the
side it was measured on — as a human label.
*/
export const metricLabel = (metric: string): string => {
	const [name, side] = metric.split(":");
	const words = name.replace(/_/g, " ");

	return side ? `${words} · ${side}` : words;
};

/*
readinessGate names the stage gate that owns a kernel. The gates are snake
cased where the measurement source is not, so the two have to be mapped rather
than assumed equal.
*/
const READINESS_GATE: Record<string, string> = {
	depthflow: "depth_flow",
	leadlag: "lead_lag",
	pumpdump: "pump_dump",
};

export const readinessGate = (source: string) =>
	READINESS_GATE[source.toLowerCase()] ?? source.toLowerCase();

export const orderedKernelSources = (sources: string[]): string[] =>
	[...sources].sort((left, right) => {
		const byOrder = kernelOrderIndex(left) - kernelOrderIndex(right);

		return byOrder === 0 ? left.localeCompare(right) : byOrder;
	});

const KERNEL_COPY: Record<
	string,
	{ name: string; sub: string; blurb: string }
> = {
	hawkes: {
		name: "Hawkes process",
		sub: "hawkes · branching η",
		blurb:
			"Self-exciting point process over order-flow events. Branching ratio η near 1 means the book is reflexive and primed to cascade.",
	},
	// "prediction" is a presentation alias for the resonance measurement: the
	// backend has one autoencoder output, while the mockup names the detail row
	// after its predictive-coding role.
	prediction: {
		name: "Predictive coding",
		sub: "predict · 8-step horizon",
		blurb:
			"Hierarchical generative model. Each layer predicts the one below; the residual error norm is the tradeable surprise.",
	},
	resonance: {
		name: "Resonance",
		sub: "resonance · laminar/turbulent",
		blurb:
			"Latent-state x-ray of coupled oscillators. Laminar phase locks ride trends; turbulent decoherence precedes reversals.",
	},
	cognitive: {
		name: "Cognitive memory",
		sub: "cognitive · entropy gate",
		blurb:
			"Discrete DMT sequence sealed through an entropy gate, then a beam search lookahead picks the winning regime class.",
	},
	causal: {
		name: "Causal ladder",
		sub: "causal · assoc→interv→cf",
		blurb:
			"Pearl do-calculus. Climbs association → intervention → counterfactual to estimate the effect of acting, not merely observing.",
	},
	manifold: {
		name: "Manifold",
		sub: "manifold · |ψ|² · particles",
		blurb:
			"Pilot-wave projection of the coherence field. |ψ|² paints the cloud, guidance current is the stream, and particles surf that stream.",
	},
	regime: {
		name: "Regime radar",
		sub: "regime · 5-axis",
		blurb:
			"Cross-section mean of volatility, trend, bullish, bearish and choppiness — the coarse weather of the tape.",
	},
	correlation: {
		name: "Correlation",
		sub: "correlation · cross-section",
		blurb: "Cross-symbol correlation pressure from backend measurements.",
	},
	derivatives: {
		name: "Derivatives flow",
		sub: "derivatives · basis / OI / liquidations",
		blurb:
			"Real-time perpetual futures positioning, basis geometry, open interest velocity, and leveraged regime dynamics.",
	},
	pumpdump: {
		name: "Pumpdump",
		sub: "pumpdump · ignition",
		blurb:
			"Pump impulse measurement from raw market frames projected into the terminal signal surface.",
	},
	leadlag: {
		name: "Lead-lag",
		sub: "leadlag · cross-lag",
		blurb:
			"Lead-lag coupling across the cross-section from backend measurements.",
	},
	liquidity: {
		name: "Liquidity",
		sub: "liquidity · touch scarcity",
		blurb:
			"Current executable touch depth relative to the market cross-section, with reported-volume turnover shown separately as context.",
	},
	toxicity: {
		name: "Toxicity",
		sub: "toxicity · flow",
		blurb: "Order-flow toxicity measurement from backend frames.",
	},
	exhaustion: {
		name: "Exhaustion",
		sub: "exhaustion · fade",
		blurb: "Exhaustion measurement from backend frames.",
	},
	depthflow: {
		name: "Depthflow",
		sub: "depthflow · ladder",
		blurb: "Depth flow measurement from backend frames.",
	},
	cvd: {
		name: "CVD",
		sub: "cvd · pressure",
		blurb: "Cumulative volume delta pressure from backend frames.",
	},
	sentiment: {
		name: "Sentiment",
		sub: "sentiment · tape",
		blurb: "Sentiment measurement from backend frames.",
	},
};

export const kernelCopy = (
	source: string,
	_category: string,
): { name: string; sub: string; blurb: string } => {
	const copy = KERNEL_COPY[source];

	if (copy === undefined) {
		throw new Error(`unsupported measurement source: ${source}`);
	}

	return copy;
};

export const kernelStatusMeta = (
	status: SignalHealthStatus,
): KernelStatusMeta => {
	const table: Record<SignalHealthStatus, KernelStatusMeta> = {
		measured: {
			label: "Healthy",
			fg: "var(--up)",
			bg: "color-mix(in srgb, var(--up) 12%, transparent)",
			bd: "color-mix(in srgb, var(--up) 38%, transparent)",
		},
		ambiguous: {
			label: "Ambig",
			fg: "var(--down)",
			bg: "color-mix(in srgb, var(--down) 12%, transparent)",
			bd: "color-mix(in srgb, var(--down) 38%, transparent)",
		},
		fault: {
			label: "Fault",
			fg: "var(--down)",
			bg: "color-mix(in srgb, var(--down) 12%, transparent)",
			bd: "color-mix(in srgb, var(--down) 38%, transparent)",
		},
		waiting: {
			label: "Standby",
			fg: "var(--f3)",
			bg: "var(--line)",
			bd: "var(--line2)",
		},
		standby: {
			label: "Standby",
			fg: "var(--f3)",
			bg: "var(--line)",
			bd: "var(--line2)",
		},
		unfocused: {
			label: "Off focus",
			fg: "var(--warn)",
			bg: "color-mix(in srgb, var(--warn) 12%, transparent)",
			bd: "color-mix(in srgb, var(--warn) 38%, transparent)",
		},
		calibrating: {
			label: "Calib",
			fg: "var(--info)",
			bg: "color-mix(in srgb, var(--info) 12%, transparent)",
			bd: "color-mix(in srgb, var(--info) 38%, transparent)",
		},
		unknown: {
			label: "No status",
			fg: "var(--warn)",
			bg: "color-mix(in srgb, var(--warn) 12%, transparent)",
			bd: "color-mix(in srgb, var(--warn) 38%, transparent)",
		},
	};

	return table[status] ?? table.waiting;
};

/*
kernelStatusVariant maps kernel health status onto semantic UI variants.
*/
export const kernelStatusVariant = (status: SignalHealthStatus): Variant => {
	switch (status) {
		case "measured":
			return "success";
		case "fault":
		case "ambiguous":
			return "error";
		case "unknown":
		case "unfocused":
			return "warning";
		case "waiting":
		case "standby":
			return "disabled";
		case "calibrating":
			return "info";
		default: {
			const _exhaustive: never = status;
			return _exhaustive;
		}
	}
};

export const kernelSparkPaths = (
	values: number[],
	status: SignalHealthStatus = "unknown",
): {
	spark: string;
	area: string;
	line: string;
	fill: string;
	active: boolean;
} => {
	const history = values.length > 0 ? values : [0];

	/*
	The y-domain is fixed so the trace scrolls instead of renormalizing: a
	window-relative min/max scale moves every existing point whenever a new
	extreme lands, which reads as the whole line jumping as one segment.
	Normalized [0,1] readings map directly, signed [-1,1] readings fold to the
	lower half, and anything outside clamps to the rails, so old points keep
	their position and only the newest point moves.
	*/
	const scaled = history.map((value) => {
		if (!Number.isFinite(value)) return 0.5;
		if (value < 0) return Math.max(0, (value + 1) / 2);
		return Math.min(1, value);
	});
	const active = status === "measured" || status === "ambiguous";
	const points = scaled.map((value, index) => {
		const x =
			scaled.length === 1
				? index === 0
					? "0.0"
					: "150.0"
				: ((index / Math.max(scaled.length - 1, 1)) * 150).toFixed(1);
		const clamped = Math.max(0, Math.min(1, value));
		const y = (29 - clamped * 26).toFixed(1);

		return `${x},${y}`;
	});
	const spark =
		scaled.length === 1
			? `${points[0]} 150,${points[0].split(",")[1]}`
			: points.join(" ");
	const area = `${spark} 150,30 0,30`;

	return {
		spark,
		area,
		line: active ? "var(--acc)" : "var(--info)",
		fill: active
			? "color-mix(in srgb, var(--acc) 16%, transparent)"
			: "color-mix(in srgb, var(--info) 12%, transparent)",
		active,
	};
};

export const formatUptime = (startedAtMs: number | null): string => {
	if (startedAtMs === null || !Number.isFinite(startedAtMs)) {
		return "—";
	}

	const totalSeconds = Math.max(
		0,
		Math.floor((Date.now() - startedAtMs) / 1000),
	);
	const minutes = Math.floor(totalSeconds / 60);
	const seconds = totalSeconds % 60;

	return `${minutes}m ${seconds}s`;
};
