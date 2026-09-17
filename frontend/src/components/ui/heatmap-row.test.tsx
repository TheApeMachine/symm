import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { HeatmapRow, HeatmapStrip } from "./heatmap-row";

describe("HeatmapStrip", () => {
	it("renders the correct number of cells with custom colors", () => {
		const values = [0.1, 0.5, 0.9];
		const markup = renderToStaticMarkup(
			<HeatmapStrip
				values={values}
				columns={16}
				colorFn={(v) => (v > 0.4 ? "rgb(255,0,0)" : "rgb(0,0,255)")}
			/>,
		);

		expect(markup).toContain("grid-cols-16");
		expect(markup).toContain("background:rgb(0,0,255)");
		expect(markup).toContain("background:rgb(255,0,0)");
	});
});

describe("HeatmapRow", () => {
	it("renders label, strip, and metric components", () => {
		const markup = renderToStaticMarkup(
			<HeatmapRow
				label="L0 · sensory"
				values={[0.2, 0.4, 0.6]}
				metric={
					<HeatmapRow.Metric
						label="ε"
						value="0.123"
						percent={12.3}
						variant="success"
					/>
				}
			/>,
		);

		expect(markup).toContain("L0 · sensory");
		expect(markup).toContain("0.123");
		expect(markup).toContain('role="progressbar"');
		expect(markup).toContain('aria-valuenow="12.3"');
	});

	it("renders within a HeatmapRow.Group", () => {
		const markup = renderToStaticMarkup(
			<HeatmapRow.Group>
				<HeatmapRow label="L0" values={[0.1]} />
				<HeatmapRow label="L1" values={[0.2]} />
			</HeatmapRow.Group>,
		);

		expect(markup).toContain("L0");
		expect(markup).toContain("L1");
	});
});
