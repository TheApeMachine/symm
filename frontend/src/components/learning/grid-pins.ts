import type { ImpulseNode } from "./types";

/** Mirrors nomagique/store/grid_pins.go and SIGNALS.md (23 regions). */
export const REGION_COUNT = 23;

export const REGION_METRIC_PINS: Record<string, Record<string, number[]>> = {
	cvd: {
		drive: [0],
		net_fraction: [0],
		balance: [1],
		starvation: [2],
		absorption: [3],
		net: [3],
		value: [0],
	},
	pumpdump: {
		compression: [0, 18],
		precursor: [0],
		rvol: [2],
		exhaustion: [3, 8, 17],
		ignition: [6],
		trend: [5],
		spread: [14],
		value: [0],
		strength: [0],
	},
	correlation: {
		herd_score: [1, 18, 20],
		strength: [1],
		peak_score: [18],
		alpha_score: [20],
		noise_score: [19],
		stress_score: [17, 19],
		correlation: [1],
		energy: [1],
	},
	leadlag: {
		inefficient_score: [1],
		sync_score: [1, 20],
		decoupled_score: [19],
		stall_score: [19],
		strength: [1],
	},
	depthflow: {
		loaded_score: [2, 5, 7],
		thin_score: [4, 8, 18, 21],
		spoof_score: [4],
		hypothesis_separation: [6],
		neutral_score: [7],
		value: [7],
	},
	toxicity: {
		support_score: [1, 7, 16, 22],
		bluff_score: [4],
		vacuum_score: [4, 18, 21],
		value: [4],
		strength: [4],
	},
	morphology: {
		bid_notional_lean: [2],
		ask_vacuum: [5],
		shape_stable: [7],
		value: [7],
		strength: [7],
	},
	hawkes: {
		spectral_radius: [6, 18],
		branching_asymmetry: [6],
		total_buy_descendants: [6],
		total_sell_descendants: [6],
		buy_arrival_rate: [7],
		sell_arrival_rate: [8],
		immediate_buy_offspring: [6],
		immediate_sell_offspring: [6],
		maturity: [18],
	},
	sentiment: {
		surge_score: [7, 18],
		breadth: [7, 18],
		slump_score: [8, 17],
		divergent_score: [19],
		change: [12],
		value: [18],
		strength: [18],
	},
	liquidity: {
		scarcity_score: [8, 13, 21],
		median_score: [14, 22],
		depth_score: [16, 22],
		value: [14],
		strength: [14],
	},
};

export const CATEGORY_NAMES = ["ENTER", "EXIT", "HOLD", "ORACLE"] as const;

export const regionCategoryIndex = (regionKey: number): number => {
	if (regionKey <= 2) return 0;
	if (regionKey <= 5) return 1;
	if (regionKey <= 8) return 2;
	return 3;
};

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
	if (regionKey <= 8) {
		return {
			x: width * (0.2 + (regionKey - 6) * 0.24),
			y: height * 0.62,
		};
	}

	const slot = Math.min(Math.max(regionKey - 9, 0), 13);
	return {
		x: width * (0.08 + (slot / 13) * 0.84),
		y: height * 0.82,
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

export const getCanonicalPinDefs = (): CanonicalPinDef[] => {
	const pins: CanonicalPinDef[] = [];

	for (let regionKey = 0; regionKey < REGION_COUNT; regionKey++) {
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

export const getCanonicalRegionCounts = (): Record<number, number> => {
	const counts: Record<number, number> = {};
	for (let i = 0; i < REGION_COUNT; i++) counts[i] = 0;
	for (const [, metricMap] of Object.entries(REGION_METRIC_PINS)) {
		for (const [, targetRegions] of Object.entries(metricMap)) {
			for (const r of targetRegions) {
				counts[r] = (counts[r] ?? 0) + 1;
			}
		}
	}
	return counts;
};
