import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it } from "vitest";
import { clockAtom, updateClock } from "#/collections/app";
import { Clock } from "#/components/clock";

describe("Clock", () => {
	beforeEach(() => {
		clockAtom.set(null);
	});

	it("renders fallback wall clock when clockAtom is null", () => {
		const html = renderToStaticMarkup(<Clock />);
		expect(html).toContain("UTC");
		expect(html).toContain("engine clock");
		expect(html).toContain("data-time");
		expect(html).toContain("data-date");
	});

	it("renders formatted engine time and date when clockAtom has a timestamp", () => {
		// 2026-09-13T20:15:30.000Z
		const ts = Date.UTC(2026, 8, 13, 20, 15, 30);
		clockAtom.set(ts);

		const html = renderToStaticMarkup(<Clock />);
		expect(html).toContain("20:15:30 UTC");
		expect(html).toContain("2026-09-13 engine clock");
	});

	it("updates correctly via updateClock with nanoseconds bigint", () => {
		// 2026-09-13T21:30:00.000Z in nanoseconds
		const tsMs = Date.UTC(2026, 8, 13, 21, 30, 0);
		const tsNs = BigInt(tsMs) * 1000000n;
		updateClock(tsNs);

		const html = renderToStaticMarkup(<Clock />);
		expect(html).toContain("21:30:00 UTC");
		expect(html).toContain("2026-09-13 engine clock");
	});

	it("updates correctly via updateClock with milliseconds number", () => {
		const tsMs = Date.UTC(2026, 8, 13, 22, 45, 15);
		updateClock(tsMs);

		const html = renderToStaticMarkup(<Clock />);
		expect(html).toContain("22:45:15 UTC");
		expect(html).toContain("2026-09-13 engine clock");
	});
});
