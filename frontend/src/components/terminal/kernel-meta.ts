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
rather than assumed. A generic "strength" was asked of every kernel, and the
ones that do not publish one — hawkes reports a fitted intensity, liquidity
scores scarcity, exhaustion and toxicity split their readings by side — showed
nothing at all.

Hawkes leads with its fitted branching ratio. Conditional intensity is measured
in events per second and has no intrinsic unit interval, while spectral radius
is the dimensionless cascade measure the row describes and already carries the
model's normalized estimate.
*/
const SOURCE_HEADLINE: Record<string, string> = {
	correlation: "snr",
	cvd: "strength",
	depthflow: "snr",
	exhaustion: "strength:buy",
	hawkes: "spectral_radius",
	leadlag: "strength",
	liquidity: "scarcity_score",
	pumpdump: "strength",
	sentiment: "strength",
	toxicity: "touch_quantity:buy",
};

/*
sourceHeadlineMetric names the metric map entry a kernel row leads with.
*/
export const sourceHeadlineMetric = (source: string) =>
	`metrics.${SOURCE_HEADLINE[source.toLowerCase()] ?? "strength"}`;

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
		"snr",
		"correlation",
		"signed",
		"relative_energy",
		"herd_score",
		"alpha_score",
		"noise_score",
		"stress_score",
	],
	cvd: [
		"strength",
		"absorption",
		"drive",
		"balance",
		"starvation",
		"net_fraction",
	],
	depthflow: [
		"snr",
		"loaded_score",
		"spoof_score",
		"thin_score",
		"neutral_score",
	],
	exhaustion: [
		"strength:buy",
		"strength:sell",
		"urgency:buy",
		"reversal:buy",
		"mechanical:buy",
		"fragile:buy",
	],
	hawkes: [
		"conditional_intensity:buy",
		"conditional_intensity:sell",
		"baseline_intensity:buy",
		"excitation_amplitude:buy_to_buy",
		"spectral_radius",
		"expected_total_descendants:buy",
	],
	leadlag: [
		"strength",
		"correlation",
		"signed_correlation",
		"lag_fraction",
		"sample_support",
	],
	liquidity: [
		"scarcity_score",
		"relative_touch_depth",
		"executable_touch_depth",
		"reported_volume_notional",
	],
	pumpdump: [
		"strength",
		"ignition",
		"compression",
		"precursor",
		"trend",
		"exhaustion",
	],
	sentiment: [
		"strength",
		"breadth",
		"change",
		"divergent_score",
		"leader_strength",
	],
	toxicity: [
		"touch_quantity:buy",
		"touch_quantity:sell",
		"cancelled_quantity:buy",
		"cancelled_quantity:sell",
		"fill_volume:buy",
		"fill_volume:sell",
	],
};

/*
sourceMetrics names the metrics a kernel's detail panel reads. A kernel with no
entry falls back to the one metric its row leads with.
*/
export const sourceMetrics = (source: string): string[] =>
	SOURCE_METRICS[source.toLowerCase()] ?? [
		sourceHeadlineMetric(source).slice("metrics.".length),
	];

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

export const kernelCopy = (source: string, category: string) =>
	KERNEL_COPY[source] ?? {
		name: source,
		sub: category || source,
		blurb: "Backend measurement projected into the terminal signal surface.",
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
	const minimum = Math.min(...history);
	const maximum = Math.max(...history);
	const scaled =
		minimum < 0 || maximum > 1
			? history.map((value) =>
					maximum > minimum
						? (value - minimum) / (maximum - minimum)
						: // ponytail: window-relative min/max scaling; upgrade path is per-kernel
							// rolling quantile normalization from measurement history.
							value > 0
							? 0.5
							: 0,
				)
			: history;
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
