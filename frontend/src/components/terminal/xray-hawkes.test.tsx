import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { XrayHawkesPanel } from "./xray-hawkes";
import { hawkesTrace } from "./xray-hawkes-trace";

describe("XrayHawkesPanel", () => {
	it("renders the arrival-process readouts and canvas shell", () => {
		const markup = renderToStaticMarkup(<XrayHawkesPanel />);

		expect(markup).toContain("<canvas");
		expect(markup).toContain('data-f="events"');
		expect(markup).toContain('data-f="lambda"');
		expect(markup).toContain('data-f="mu"');
		expect(markup).toContain('data-f="sells"');
		expect(markup).toContain('data-f="eta"');
		expect(markup).toContain('class="absolute inset-0"');
	});
});

describe("hawkesTrace", () => {
	it("draws an instantaneous event jump followed by fitted exponential decay", () => {
		const trace = hawkesTrace(
			[
				{
					at: 0n,
					intensity: 0.2,
					postArrival: 0.7,
					baseline: 0.2,
					decay: 1,
				},
				{
					at: 1_000_000_000n,
					intensity: 0.38,
					postArrival: 0.88,
					baseline: 0.2,
					decay: 1,
				},
			],
			10,
		);

		expect(trace[0]).toEqual({ at: 0n, intensity: 0.2 });
		expect(trace[1]).toEqual({ at: 0n, intensity: 0.7 });
		expect(trace.at(-2)?.at).toBe(1_000_000_000n);
		expect(trace.at(-2)?.intensity).toBeCloseTo(0.38);
		expect(trace.at(-1)).toEqual({
			at: 1_000_000_000n,
			intensity: 0.88,
		});
		expect(trace[2]?.intensity).toBeLessThan(0.7);
		expect(trace[2]?.intensity).toBeGreaterThan(0.2);
	});
});
