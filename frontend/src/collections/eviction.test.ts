import { describe, expect, it } from "vitest";
import {
	DEFAULT_FOCUS_SYMBOL,
	evictStaleSymbols,
	evictSymbol,
	getCachedSymbolCount,
	getMeasurementStore,
	getResonanceReadingStore,
	MAX_CACHED_SYMBOLS,
	resonanceStore,
	signals,
	symbolsAtom,
} from "./app";

describe("Store Eviction and LRU Retention", () => {
	it("evictSymbol purges measurement and resonance stores for an unobserved symbol", () => {
		const testSymbol = "TEST-PURGE/USD";
		const store = getMeasurementStore("cvd", testSymbol);
		expect(store).toBeDefined();

		const resStore = getResonanceReadingStore(testSymbol);
		expect(resStore).toBeDefined();

		expect(signals.cvd.state[testSymbol]).toBeDefined();
		expect(resonanceStore.state[testSymbol]).toBeDefined();

		evictSymbol(testSymbol);

		expect(signals.cvd.state[testSymbol]).toBeUndefined();
		expect(resonanceStore.state[testSymbol]).toBeUndefined();
	});

	it("prunes evicted symbols from symbolsAtom while retaining default focus", () => {
		const symbolA = "SYM-A/USD";
		symbolsAtom.set([DEFAULT_FOCUS_SYMBOL, symbolA]);

		evictSymbol(symbolA);
		expect(symbolsAtom.get()).not.toContain(symbolA);
		expect(symbolsAtom.get()).toContain(DEFAULT_FOCUS_SYMBOL);

		evictSymbol(DEFAULT_FOCUS_SYMBOL);
		expect(symbolsAtom.get()).toContain(DEFAULT_FOCUS_SYMBOL);
	});

	it("bounds total cached symbols when exceeding capacity", () => {
		for (let i = 0; i < MAX_CACHED_SYMBOLS + 10; i++) {
			getMeasurementStore("cvd", `SYM-${i}/USD`);
		}

		expect(getCachedSymbolCount()).toBeLessThanOrEqual(MAX_CACHED_SYMBOLS);
	});

	it("evictStaleSymbols drops symbols older than maxAgeMs", () => {
		const staleSym = "STALE/USD";
		getMeasurementStore("cvd", staleSym);

		evictStaleSymbols(0);

		expect(signals.cvd.state[staleSym]).toBeUndefined();
	});
});
