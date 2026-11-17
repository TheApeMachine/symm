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

	it("renders a sparkline and meter for the kernel row", () => {
		const { container } = render(<Metric name="cvd" />);

		expect(container.querySelector('[data-k="spark"]')).not.toBeNull();
		expect(container.querySelector('[role="progressbar"]')).not.toBeNull();
	});

	it("lets the sparkline shrink so the row can flex instead of overflowing", () => {
		const { container } = render(<Metric name="cvd" />);
		const spark = container.querySelector("svg");
		const button = container.querySelector("button");

		expect(spark?.getAttribute("class")).toContain("flex-1");
		expect(spark?.getAttribute("class")).toContain("min-h-0");
		expect(button?.className).toContain("min-h-0");
		expect(button?.className).toContain("justify-start");
	});
});
