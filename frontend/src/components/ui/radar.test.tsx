import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Radar } from "./radar";

const sampleAxes = [
	{ label: "volatility", x: 0, y: -1 },
	{ label: "trend", x: 0.951, y: -0.309 },
	{ label: "drive", x: 0.588, y: 0.809 },
	{ label: "starved", x: -0.588, y: 0.809 },
	{ label: "chop", x: -0.951, y: -0.309 },
];

describe("Radar component", () => {
	it("renders polygon levels, spokes, and data-axis arms", () => {
		const markup = renderToStaticMarkup(<Radar axes={sampleAxes} />);

		const arms = [...markup.matchAll(/data-axis="([^"]+)"/g)].map((m) => m[1]);
		expect(arms).toHaveLength(5);
		expect(arms).toEqual(["volatility", "trend", "drive", "starved", "chop"]);

		expect(markup).toContain("<svg");
		expect(markup).toContain("Regime radar");
		expect(markup).toContain("volatility");
		expect(markup).toContain("chop");
	});

	it("supports custom title and variants", () => {
		const markup = renderToStaticMarkup(
			<Radar axes={sampleAxes} title="Custom Matrix" variant="brand" size="s" />,
		);

		expect(markup).toContain("Custom Matrix");
		expect(markup).toContain("[--radar-arm:var(--brand)]");
		expect(markup).toContain("max-w-[180px]");
	});
});
