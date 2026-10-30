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
	"depthflow",
	"liquidity",
	"training",
] as const;

const kernelOrderIndex = (source: string): number => {
	const index = TERMINAL_KERNEL_ORDER.indexOf(
		source as (typeof TERMINAL_KERNEL_ORDER)[number],
	);

	return index === -1 ? TERMINAL_KERNEL_ORDER.length : index;
};

/*
Headlines name one live metric per signal kernel (SIGNALS.md nine-pin grid +
oracles). Dormant booter sources (toxicity, cvd, leadlag, morphology) are
not listed here.
*/
const SOURCE_HEADLINE: Record<string, string> = {
	correlation: "structural_alignment",
	depthflow: "bid_ladder_build",
	hawkes: "supercritical_cascade",
	liquidity: "relative_spread",
	pumpdump: "coiled_compression",
	training: "confidence",
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

const SOURCE_METRICS: Record<string, string[]> = {
	correlation: [
		"structural_alignment",
		"structure_uncoupling",
	],
	depthflow: [
		"bid_ladder_build",
		"bid_ladder_release",
		"ask_ladder_release",
		"board_bid_release",
	],
	hawkes: ["supercritical_cascade", "board_cascade"],
	liquidity: [
		"best_price:bid",
		"best_price:ask",
		"relative_spread",
		"market_friction",
		"capacity_stress",
	],
	pumpdump: [
		"coiled_compression",
		"rejection_tick",
		"starved_tape",
		"blowoff_dip",
		"board_coiled",
	],
	training: [
		"steps",
		"decisions",
		"resolved",
		"confidence",
		"contrast",
		"surprisal",
		"ambiguity",
		"action",
		"win_rate",
		"edge",
		"progress",
		"accuracy",
		"support",
		"quality",
	],
};

/*
sourceMetrics names the metrics a kernel's detail panel reads. An unsupported
source is a wiring error; silently painting a generic metric would make that
frontend defect indistinguishable from missing backend data.
*/
export const sourceMetrics = (source: string): string[] => {
	const metrics = SOURCE_METRICS[source.toLowerCase()];

	if (metrics === undefined) {
		throw new Error(`unsupported measurement source: ${source}`);
	}

	return metrics;
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
	liquidity: {
		name: "Liquidity",
		sub: "liquidity · touch scarcity",
		blurb:
			"Current executable touch depth relative to the market cross-section, with reported-volume turnover shown separately as context.",
	},
	depthflow: {
		name: "Depthflow",
		sub: "depthflow · ladder",
		blurb: "Depth flow measurement from backend frames.",
	},
	training: {
		name: "Cognitive Training",
		sub: "training · precursor radix",
		blurb:
			"Precursor associative learning model compiling excursions into radix basins.",
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
		line: active ? "var(--acc)" : "var(--f4)",
		fill: active
			? "color-mix(in srgb, var(--acc) 16%, transparent)"
			: "color-mix(in srgb, var(--f4) 10%, transparent)",
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
