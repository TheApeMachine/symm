import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { RingBuffer, signals } from "#/collections/app";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";
import { Positions } from "./positions";

describe("Positions", () => {
	it("renders without error when signals.positions is in default fallback state", () => {
		const markup = renderToStaticMarkup(<Positions />);
		expect(markup).toContain("no open positions");
	});

	it("renders without error when signals.positions state does not have findLast function", () => {
		const originalState = signals.positions.state;
		try {
			signals.positions.setState(() => ({}));
			const markup = renderToStaticMarkup(<Positions />);
			expect(markup).toContain("no open positions");
		} finally {
			signals.positions.setState(() => originalState);
		}
	});

	it("renders active open positions when signals.positions contains positions", () => {
		const originalState = signals.positions.state;
		try {
			const m = new MeasurementT();
			m.symbol = "NMR/USD";
			m.metrics = [
				new MetricT("pnl", 0.42),
				new MetricT("entry_price", 12.3456),
				new MetricT("mark", 12.5678),
				new MetricT("return_pct", 3.41),
			];
			const ring = new RingBuffer<MeasurementT>(1);
			ring.add(m);

			signals.positions.setState(() => ({ "NMR/USD": ring }));
			const markup = renderToStaticMarkup(<Positions />);
			expect(markup).toContain("NMR/USD");
			expect(markup).toContain("active");
			expect(markup).toContain("0.4200 USD");
			expect(markup).toContain("EXIT");
			expect(markup).toContain("12.345600");
			expect(markup).toContain("12.567800");
			expect(markup).not.toContain("no open positions");
		} finally {
			signals.positions.setState(() => originalState);
		}
	});
});
