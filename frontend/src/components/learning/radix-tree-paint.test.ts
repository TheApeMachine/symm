import { describe, expect, it } from "vitest";
import {
	formatExploreThreshold,
	formatTrieLinkToken,
} from "./radix-tree-paint";

describe("formatExploreThreshold", () => {
	it("uses two decimal places at the desk baseline", () => {
		expect(formatExploreThreshold(0.5)).toBe("0.50");
		expect(formatExploreThreshold(0.45)).toBe("0.45");
		expect(formatExploreThreshold(0.8)).toBe("0.80");
	});
});

describe("formatTrieLinkToken", () => {
	it("leaves region ids unchanged", () => {
		expect(formatTrieLinkToken("R01")).toBe("R01");
		expect(formatTrieLinkToken("[R00-R03]")).toBe("[R00-R03]");
	});

	it("formats signed magnitude edges to two decimals", () => {
		expect(formatTrieLinkToken("±0.001")).toBe("±0.00");
		expect(formatTrieLinkToken("±0.023")).toBe("±0.02");
		expect(formatTrieLinkToken("±0.005")).toBe("±0.01");
	});
});
