// @vitest-environment jsdom
import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { ResonanceCanvas } from "./resonance-canvas";
it("shows native prediction layers and resolved outcomes only when the estimator supplies them", () => {
	const view = render(<ResonanceCanvas />);
	expect(screen.getByText("Awaiting a complete metric cut")).toBeTruthy();
	view.rerender(
		<ResonanceCanvas
			reading={{
				latent: [0.2, -0.3],
				layers: [{ state: [0.1, -0.2], prediction: [0.15, -0.1] }],
				skillAverage: 0.9,
				skillReadyAvg: false,
				precisionAverage: 2,
				precisionReadyAvg: false,
				surprise: 0.3,
				energy: 0.4,
			}}
			forwardCurve={[0.1, 0.2]}
			supportedHorizon="2"
			calibrated={true}
			resolvedSteps="12"
			lastResolution={{ prediction: 0.1, target: 1, error: 0.9 }}
		/>,
	);
	expect(screen.getByText("settled")).toBeTruthy();
	expect(screen.getByText("Awaiting resolved forecasts")).toBeTruthy();
	expect(screen.getByText("12")).toBeTruthy();
	expect(screen.queryByText("2.000")).toBeNull();
});
