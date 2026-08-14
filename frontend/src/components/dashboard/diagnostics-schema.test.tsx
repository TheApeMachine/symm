import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
	averageNanos,
	DiagnosticsSchema,
	formatAge,
	formatNanos,
} from "./diagnostics-schema";

describe("formatNanos", () => {
	it("renders sub-millisecond hops without collapsing them to zero", () => {
		expect(formatNanos(0)).toBe("—");
		expect(formatNanos(480)).toBe("480ns");
		expect(formatNanos(12_400)).toBe("12.4µs");
		expect(formatNanos(2_500_000)).toBe("2.50ms");
	});
});

describe("formatAge", () => {
	it("distinguishes never, now, and elapsed", () => {
		expect(formatAge(0, false)).toBe("never");
		expect(formatAge(0, true)).toBe("now");
		expect(formatAge(2_000_000_000, true)).toBe("2.00s");
	});
});

describe("averageNanos", () => {
	it("divides total time by the observed count", () => {
		expect(averageNanos({ count: 4, total_ns: 400 })).toBe(100);
		expect(averageNanos({ count: 0, total_ns: 400 })).toBe(0);
	});
});

describe("DiagnosticsSchema", () => {
	it("labels each named module and inbound wait", () => {
		const markup = renderToStaticMarkup(
			<DiagnosticsSchema
				stages={[
					{
						name: "crypto",
						kind: "trader",
						count: 2,
						total_ns: 4_000,
						last_ns: 2_000,
					},
					{
						name: "depthflow",
						kind: "signal",
						count: 4,
						total_ns: 8_000_000,
						last_ns: 2_000_000,
					},
					{
						name: "planner",
						kind: "strategy",
						count: 1,
						total_ns: 5_000_000,
						last_ns: 5_000_000,
					},
				]}
				hops={[
					{
						from: "crypto",
						to: "depthflow",
						count: 4,
						total_ns: 1_600,
						last_ns: 400,
					},
				]}
			/>,
		);

		expect(markup).toContain("Crypto");
		expect(markup).toContain("Depthflow");
		expect(markup).toContain("Planner");
		expect(markup).toContain("400ns");
	});
});
