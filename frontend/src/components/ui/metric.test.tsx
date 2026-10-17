// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Metric } from "./metric";

describe("Metric layout", () => {
	afterEach(() => {
		cleanup();
	});

	it("shares leftover pane height instead of standing at 100% of the parent", () => {
		const { container } = render(<Metric name="cvd" />);
		const root = container.firstElementChild;

		expect(root?.className).toContain("flex-1");
		expect(root?.className).toContain("min-h-0");
		expect(root?.className.split(/\s+/)).not.toContain("h-full");
	});

	it("names the headline metric so DRAW can paint the sparkline and meter", () => {
		const { container } = render(<Metric name="cvd" />);

		expect(
			container.querySelector('[data-k="sparkline"]')?.getAttribute("data-series"),
		).toBe("signed_net_fraction");
		expect(
			container.querySelector("[data-metric-meter]")?.getAttribute("data-metric-meter"),
		).toBe("signed_net_fraction");
		expect(
			container.querySelector("[data-metric-badge]")?.getAttribute("data-metric-badge"),
		).toBe("signed_net_fraction");
	});

	it("lets the sparkline shrink so the row can flex instead of overflowing", () => {
		const { container } = render(<Metric name="cvd" />);
		const spark = container.querySelector('[data-k="sparkline"]');
		const button = container.querySelector("button");

		expect(spark?.getAttribute("class")).toContain("flex-1");
		expect(spark?.getAttribute("class")).toContain("min-h-0");
		expect(button?.className).toContain("min-h-0");
		expect(button?.className).toContain("justify-start");
	});
});
