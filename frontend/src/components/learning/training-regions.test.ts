import { describe, expect, it } from "vitest";
import { Value } from "#/types/value";
import {
	regionMapsFromTrainingFrame,
	regionPinLabel,
} from "./training-regions";

describe("regionMapsFromTrainingFrame", () => {
	it("falls back to lit regions when region_brightness metrics are absent", () => {
		const frame = new Value({
			scope: "BTC/USD",
			origin: "training",
			prefix: "training/step/BTC-USD",
		});
		frame.layers = [
			new TextEncoder().encode(
				JSON.stringify([
					{ key: 6, brightness: 2.1, counts: 4 },
					{ key: 1, brightness: 0.4, counts: 2 },
				]),
			),
		];
		frame.index = [`${frame.prefix}/lit`];

		const maps = regionMapsFromTrainingFrame(frame);
		expect(maps.brightness.R06).toBe(2.1);
		expect(maps.members.R06).toBe(4);
		expect(maps.brightness.R01).toBe(0.4);
		expect(maps.members.R01).toBe(2);
	});

	it("labels pinned wiring separately from live reporting counts", () => {
		const maps = regionMapsFromTrainingFrame(null);
		expect(regionPinLabel("R01", maps)).toBe("2 metrics pinned");

		maps.brightness.R01 = 1.2;
		expect(regionPinLabel("R01", maps)).toBe("2 metrics pinned");

		maps.members.R01 = 1;
		expect(regionPinLabel("R01", maps)).toBe("1/2 reporting");
	});
});
