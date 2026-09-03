import { describe, expect, it } from "vitest";
import {
	readClass,
	readRole,
	readStance,
	readSupport,
	summarise,
	UNDECLARED,
} from "./reading";

describe("readRole", () => {
	it("restates a declared role in plain language", () => {
		const reading = readRole("ARRIVAL_MODEL_STATE");

		expect(reading?.title).toBe("Event timing");
		expect(reading?.plain).toContain("arriving in time");
	});

	it("passes an undeclared role through rather than guessing at it", () => {
		const reading = readRole("SOME_FUTURE_ROLE");

		expect(reading?.title).toBe("SOME_FUTURE_ROLE");
		expect(reading?.plain).toBe("");
	});

	it("has nothing to say where no role was declared", () => {
		expect(readRole(undefined)).toBeNull();
	});
});

describe("readClass", () => {
	it("explains how a standardised comparison should be read", () => {
		expect(readClass("standardized_historical_comparison")?.plain).toContain(
			"standard deviations",
		);
	});

	it("passes an undeclared class through", () => {
		expect(readClass("not_a_class")?.title).toBe("not_a_class");
	});
});

describe("readSupport", () => {
	it("reads full maturity above its noise as well supported", () => {
		const support = readSupport(0.9, 3);

		expect(support.tone).toBe("strong");
		expect(support.label).toBe("well supported");
	});

	it("reads partial maturity as provisional", () => {
		expect(readSupport(0.5, 2).tone).toBe("fair");
	});

	it("reads a quantity at or below its own noise as weak, however mature", () => {
		const support = readSupport(1, 0.8);

		expect(support.tone).toBe("weak");
		expect(support.label).toBe("at or below noise");
	});

	it("reads thin maturity as weak where no noise was defined", () => {
		const support = readSupport(0.1, null);

		expect(support.tone).toBe("weak");
		expect(support.label).toBe("thin support");
	});

	it("never judges the market, only the estimate", () => {
		for (const maturity of [0, 0.5, 1]) {
			for (const snr of [null, 0.5, 5]) {
				const support = readSupport(maturity, snr);

				expect(support.plain).not.toMatch(/bullish|bearish|buy|sell|good|bad/i);
			}
		}
	});
});

describe("readStance", () => {
	it("names a supporting stance as an argument for", () => {
		expect(readStance("supports").tone).toBe("up");
		expect(readStance("supports").label).toBe("argued for");
	});

	it("names a contradicting stance as an argument against", () => {
		expect(readStance("contradicts").tone).toBe("down");
	});

	it("names missing evidence as wanted rather than absent", () => {
		expect(readStance("missing").tone).toBe("muted");
		expect(readStance("missing").label).toBe("wanted, not available");
	});
});

describe("summarise", () => {
	it("quotes a declared purpose", () => {
		expect(
			summarise({
				identity: "hawkes/intensity",
				source: "hawkes",
				metric: "intensity",
				purpose: "Conditional arrival intensity.",
			}),
		).toBe("Conditional arrival intensity.");
	});

	it("says an undeclared metric is unexplained rather than inventing a meaning", () => {
		expect(summarise(null)).toBe(UNDECLARED);
		expect(
			summarise({
				identity: "x/y",
				source: "x",
				metric: "y",
				purpose: "   ",
			}),
		).toBe(UNDECLARED);
	});
});
