import type { Value } from "#/types/value";
import { getCanonicalRegionCounts } from "./grid-pins";

const PINNED_METRICS_BY_REGION = getCanonicalRegionCounts();

type WireRegion = {
	key?: number;
	brightness?: number;
	counts?: number;
};

export type TrainingRegionMaps = {
	brightness: Record<string, number | undefined>;
	members: Record<string, number | undefined>;
};

const regionKeyFromIndex = (index: number): string =>
	`R${String(index).padStart(2, "0")}`;

/*
regionMapsFromTrainingFrame merges explicit region_brightness metrics with the
live lit/active layers the engine publishes on every training step.
*/
export function regionMapsFromTrainingFrame(
	frame: Value | null,
): TrainingRegionMaps {
	const brightness: Record<string, number | undefined> = {};
	const members: Record<string, number | undefined> = {};

	if (!frame) {
		return { brightness, members };
	}

	for (const [suffix, value] of Object.entries(frame.metrics())) {
		if (suffix.startsWith("region_brightness:")) {
			brightness[suffix.slice("region_brightness:".length)] = value;
		}
		if (suffix.startsWith("region_members:")) {
			members[suffix.slice("region_members:".length)] = value;
		}
	}

	const ingest = (rows: WireRegion[] | null | undefined) => {
		if (!rows) return;
		for (const row of rows) {
			if (row.key === undefined || typeof row.brightness !== "number") {
				continue;
			}
			const key = regionKeyFromIndex(row.key);
			if (brightness[key] === undefined) {
				brightness[key] = row.brightness;
			}
			if (members[key] === undefined && typeof row.counts === "number") {
				members[key] = row.counts;
			}
		}
	};

	ingest(frame.reading("lit") as WireRegion[] | null);
	ingest(frame.reading("active") as WireRegion[] | null);

	return { brightness, members };
}

/*
regionPinLabel separates wiring (always two pinned metrics per R00–R14) from
how many of those metrics reported on this tick (grid Region.counts).
*/
export function regionPinLabel(
	regionKey: string,
	maps: TrainingRegionMaps,
): string {
	const regionIndex = Number(regionKey.slice(1));
	const pinned = PINNED_METRICS_BY_REGION[regionIndex] ?? 0;
	const brightness = maps.brightness[regionKey];

	if (brightness === undefined) {
		return pinned > 0 ? `${pinned} metrics pinned` : "no tick yet";
	}

	const live = maps.members[regionKey];
	if (live !== undefined && Number.isFinite(live)) {
		const reporting = Math.round(live);
		return `${reporting}/${pinned} reporting`;
	}

	return `${pinned} metrics pinned`;
}
