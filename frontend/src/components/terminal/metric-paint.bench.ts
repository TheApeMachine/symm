import { bench, describe } from "vitest";
import type { Measurement } from "#/collections/types";
import { mergeInspectorMetrics } from "./inspector-meters";

const measurement = (at: string, raw: number): Measurement => ({
	source: "depthflow",
	symbol: "BTC/USD",
	at,
	validity: { state: "valid", readiness: "observation" },
	scale: { kind: "observation_window", from: at, through: at },
	metrics: {
		loaded_score: { raw: raw * 0.27, normalized: raw * 0.27 },
		spoof_score: { raw: raw * 0.18, normalized: raw * 0.18 },
		thin_score: { raw: raw * 0.7, normalized: raw * 0.7 },
		neutral_score: { raw, normalized: raw },
		strength: { raw: raw * 0.91, normalized: raw * 0.91 },
	},
});

describe("inspector metric paint", () => {
	const at = "2026-07-14T16:25:24Z";
	const values = [
		measurement(at, 0.12),
		measurement(at, 0.31),
		measurement(at, 0.44),
		measurement(at, 0.07),
		measurement(at, 0.22),
	];

	bench("merges one observation tick", () => {
		mergeInspectorMetrics(values, "depthflow", "BTC/USD");
	});
});
