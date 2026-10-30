import type { ImpulseNode } from "./types";

/** Metric → region indices; mirrors nomagique/store/grid_pins.go and SIGNALS.md. */
export const REGION_METRIC_PINS: Record<string, Record<string, number[]>> = {
	pumpdump: {
		coiled_compression: [0],
		rejection_tick: [3],
		starved_tape: [2],
		blowoff_dip: [8],
		board_coiled: [9],
	},
	correlation: {
		structural_alignment: [1],
		structure_uncoupling: [13],
	},
	depthflow: {
		ask_ladder_release: [5],
		bid_ladder_build: [7],
		bid_ladder_release: [4],
		board_bid_release: [11],
	},
	hawkes: {
		supercritical_cascade: [6],
		board_cascade: [10],
	},
	liquidity: {
		market_friction: [12],
		capacity_stress: [14],
	},
};

export const CATEGORY_NAMES = [
	"ENTER",
	"EXIT",
	"REGIME",
	"HISTORY",
	"RISK",
] as const;

export const regionCategoryIndex = (regionKey: number): number => {
	if (regionKey <= 2) return 0;
	if (regionKey <= 5) return 1;
	if (regionKey <= 9) return 2;
	if (regionKey <= 11) return 3;
	return 4;
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
	/** When set, layers live under kernel/aggregate/… instead of kernel/symbol/… */
	scope?: "aggregate";
}

const AGGREGATE_METRICS: Record<string, Set<string>> = {
	correlation: new Set(["structural_alignment", "structure_uncoupling"]),
	pumpdump: new Set(["board_coiled"]),
	depthflow: new Set(["board_bid_release"]),
	hawkes: new Set(["board_cascade"]),
	liquidity: new Set(["market_friction", "capacity_stress"]),
};

const pinScope = (source: string, metric: string): "aggregate" | undefined =>
	AGGREGATE_METRICS[source]?.has(metric) ? "aggregate" : undefined;

export const getCanonicalPinDefs = (): CanonicalPinDef[] => {
	const pins: CanonicalPinDef[] = [];

	for (let regionKey = 0; regionKey < 15; regionKey++) {
		const regionId = `R${String(regionKey).padStart(2, "0")}`;
		const category = regionCategoryIndex(regionKey);
		const categoryName = CATEGORY_NAMES[category];

		for (const [source, metricMap] of Object.entries(REGION_METRIC_PINS)) {
			for (const [metric, targetRegions] of Object.entries(metricMap)) {
				if (targetRegions.includes(regionKey)) {
					const scope = pinScope(source, metric);
					pins.push({
						id: `${source}:${metric}:${regionId}`,
						source,
						metric,
						cluster: regionKey,
						regionId,
						category,
						categoryName,
						...(scope ? { scope } : {}),
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
