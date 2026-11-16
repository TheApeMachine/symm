import { describe, expect, it } from "vitest";
import {
	isSymbolKeyedMap,
	normalizeKernelStoreValue,
} from "#/hub/signal-envelope";

describe("normalizeKernelStoreValue", () => {
	it("wraps a focused pumpdump reading", () => {
		const reading = { compression: 0.4, precursor: 0.8 };
		expect(normalizeKernelStoreValue("pumpdump", reading, "BTC/USD")).toEqual({
			"BTC/USD": reading,
		});
	});

	it("leaves ui.Focus symbol-keyed maps merged", () => {
		const wire = {
			"BTC/USD": { compression: 0.4 },
			"ETH/USD": { compression: 0.1 },
		};
		expect(normalizeKernelStoreValue("pumpdump", wire, "BTC/USD")).toBe(wire);
	});

	it("passes through non-kernel keys", () => {
		expect(normalizeKernelStoreValue("paths", { a: 1 }, "BTC/USD")).toEqual({
			a: 1,
		});
	});
});

describe("isSymbolKeyedMap", () => {
	it("detects trading-pair keys", () => {
		expect(isSymbolKeyedMap({ "BTC/USD": {} })).toBe(true);
		expect(isSymbolKeyedMap({ compression: 0.5 })).toBe(false);
	});
});