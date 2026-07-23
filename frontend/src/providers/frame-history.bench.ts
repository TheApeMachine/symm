import { bench, describe } from "vitest";
import { FrameHistory } from "#/providers/frame-history";

const symbols = Array.from({ length: 24 }, (_, index) => `SYM${index}/USD`);
const sources = [
	"causal",
	"cvd",
	"depthflow",
	"hawkes",
	"leadlag",
	"liquidity",
	"pumpdump",
	"sentiment",
	"toxicity",
];
const frames = Array.from({ length: 32 }, (_, tick) =>
	symbols.flatMap((symbol) =>
		sources.map((source) => ({
			at: new Date(Date.UTC(2026, 6, 21, 10, 0, tick)).toISOString(),
			metric: "strength",
			raw: tick,
			source,
			symbol,
		})),
	),
);

describe("FrameHistory", () => {
	bench("retains a realistic cross-section of measurement ticks", () => {
		const history = new FrameHistory(
			() => 640,
			() => "SYM0/USD",
		);

		for (const frame of frames) {
			history.retain("measurements", frame);
		}
	});

	bench("retains and projects an active measurement-history surface", () => {
		const history = new FrameHistory(
			() => 640,
			() => "SYM0/USD",
		);

		for (const frame of frames) {
			history.retain("measurements", frame);
			history.project("measurements", "history");
		}
	});
});
