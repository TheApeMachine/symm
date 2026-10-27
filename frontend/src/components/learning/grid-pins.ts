import type { ImpulseNode } from "./types";

/*
Explicit mapping between individual signal metrics and their confluence regions (0..14),
directly matching nomagique/store/grid_pins.go.
*/
export const REGION_METRIC_PINS: Record<string, Record<string, number[]>> = {
	correlation: {
		cohort_covariance_score: [13],
		historical_path_distance: [10],
		historical_path_percentile: [10],
	},
	cvd: {
		midpoint_log_return: [0],
		flow_aligned_midpoint_return: [3],
		gross_notional: [2],
		signed_net_fraction: [1, 5],
	},
	depthflow: {
		mutation_activity_imbalance: [8],
		book_imbalance: [1, 6],
		"net_displayed_flow:bid": [1, 5],
		"touch_notional:bid": [14],
		"touch_notional:ask": [2, 14],
		touch_imbalance: [1, 5],
	},
	hawkes: {
		"arrival_rate:buy": [0, 7],
		"arrival_rate:sell": [3, 4],
		branching_spectral_radius: [1, 7],
		conditional_intensity_velocity: [0, 3],
		conditional_intensity: [],
		background_rate: [],
		excitation_decay: [],
	},
	leadlag: {
		led_peer_share: [1, 7],
		covariance_score_gain_median: [7],
		best_lag_seconds_median: [6, 13],
		best_lag_seconds_mad: [8, 13],
		defined_peer_count: [],
	},
	liquidity: {
		"best_price:bid": [],
		"best_price:ask": [],
		relative_spread: [6, 8, 12],
		fee_rate: [12],
		friction_hurdle: [12],
	},
	morphology: {
		"concentration:ask": [2],
		"concentration:bid": [5, 14],
		"entropy:bid": [5, 6],
		"entropy:ask": [6, 14],
		book_shape_distance: [6, 8],
		book_shape_ks: [8],
		morphology_change: [4, 8],
	},
	pumpdump: {
		volume_rate: [0, 7],
		volume_ratio: [0, 7],
		trade_rate: [0, 9],
		positive_midpoint_return: [0],
		negative_midpoint_return: [3],
		spread_ratio: [6, 8],
	},
	sentiment: {
		same_direction_peer_count: [0, 1],
		opposite_direction_peer_count: [2, 13],
		breadth: [13],
		breadth_velocity: [8, 13],
		historical_path_distance: [11],
		historical_path_percentile: [11],
	},
	toxicity: {
		"retreat_fraction:bid": [2, 4],
		"net_withdrawal_rate:bid": [4],
	},
	hold: {
		trade_rate_ratio: [9],
		bars_held: [9],
		giveback_from_peak: [3],
		evaporation_index: [4],
	},
};

/* 5 Macro Confluence Categories matching SIGNALS.md. */
export const CATEGORY_NAMES = [
	"ENTER",
	"EXIT",
	"REGIME",
	"HISTORY",
	"RISK",
] as const;

export const regionCategoryIndex = (regionKey: number): number => {
	if (regionKey <= 2) return 0; // Enter: R00, R01, R02
	if (regionKey <= 5) return 1; // Exit: R03, R04, R05
	if (regionKey <= 9) return 2; // Regime oracle: R06–R09
	if (regionKey <= 11) return 3; // History oracle: R10, R11
	return 4; // Risk oracle: R12–R14
};

/*
sympathyAnchor places a region in the sympathy view the way SIGNALS.md
is written: enter down the left, exit down the right, and the three
oracles along the bottom in regime, history, risk order.
*/
export const sympathyAnchor = (
	regionKey: number,
	width: number,
	height: number,
): { x: number; y: number } => {
	if (regionKey <= 2) {
		return { x: width * 0.38, y: height * (0.28 + regionKey * 0.14) };
	}
	if (regionKey <= 5) {
		return {
			x: width * 0.78,
			y: height * (0.22 + (regionKey - 3) * 0.16),
		};
	}
	const slot = Math.min(Math.max(regionKey - 6, 0), 8);
	return {
		x: width * (0.14 + (slot / 8) * 0.72),
		y: height * 0.78,
	};
};

export interface CanonicalPinDef {
	id: string;
	source: string;
	metric: string;
	cluster: number;
	regionId: string;
	category: number;
	categoryName: string;
}

/*
Builds the complete, canonical list of region-metric pins in stable region order.
*/
export const getCanonicalPinDefs = (): CanonicalPinDef[] => {
	const pins: CanonicalPinDef[] = [];

	// Order by region key 0..14 so grid nodes are grouped logically
	for (let regionKey = 0; regionKey < 15; regionKey++) {
		const regionId = `R${String(regionKey).padStart(2, "0")}`;
		const category = regionCategoryIndex(regionKey);
		const categoryName = CATEGORY_NAMES[category];

		for (const [source, metricMap] of Object.entries(REGION_METRIC_PINS)) {
			for (const [metric, targetRegions] of Object.entries(metricMap)) {
				if (targetRegions.includes(regionKey)) {
					pins.push({
						id: `${source}:${metric}:${regionId}`,
						source,
						metric,
						cluster: regionKey,
						regionId,
						category,
						categoryName,
					});
				}
			}
		}
	}

	return pins;
};

/*
Creates baseline ImpulseNode items for all canonical pins.
*/
export const createCanonicalGridNodes = (): ImpulseNode[] => {
	const pins = getCanonicalPinDefs();
	return pins.map((pin) => ({
		id: pin.id,
		label: `${pin.source}:${pin.metric}`,
		cluster: pin.cluster,
		snr: 1.0,
		activation: 0.0,
	}));
};

/*
Member count per region in the canonical grid.
*/
export const getCanonicalRegionCounts = (): Record<number, number> => {
	const counts: Record<number, number> = {};
	for (let i = 0; i < 15; i++) counts[i] = 0;
	for (const [, metricMap] of Object.entries(REGION_METRIC_PINS)) {
		for (const [, targetRegions] of Object.entries(metricMap)) {
			for (const r of targetRegions) {
				counts[r] = (counts[r] ?? 0) + 1;
			}
		}
	}
	return counts;
};
