import { bench, describe } from "vitest";
import type { Measurement } from "#/types/measurement";
import { dedupeEpoch, latestEpoch, orderedEpoch } from "./measurement-view";

const measurement = (metric: string, at: string, raw: number): Measurement => ({
	source: "depthflow",
	metric,
	symbol: "BTC/USD",
	at,
	raw,
	normalized: raw,
	uncertainty: null,
	validity: { state: "valid", readiness: "observation" },
	scale: { kind: "observation_window", from: at, through: at },
});

describe("orderedEpoch", () => {
	const at = "2026-07-14T16:25:24Z";
	const values = [
		measurement("loaded_score", at, 0.12),
		measurement("spoof_score", at, 0.08),
		measurement("thin_score", at, 0.31),
		measurement("thin_score", at, 0.44),
		measurement("neutral_score", at, 0.44),
		measurement("strength", at, 0.07),
		measurement("value", at, 0.11),
		measurement("value", at, 0.22),
	];

	bench("dedupes and orders one observation tick", () => {
		orderedEpoch(values, "strength");
		dedupeEpoch(latestEpoch(values));
	});
});
