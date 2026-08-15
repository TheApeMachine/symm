import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { XrayManifoldPanel } from "./xray-manifold-panel";

describe("XrayManifoldPanel", () => {
	it("binds forecast telemetry only to readiness-aware wire fields", () => {
		const markup = renderToStaticMarkup(<XrayManifoldPanel />);

		expect(markup).toContain('data-paint="taskForecast"');
		expect(markup).toContain('data-paint="taskScale"');
		expect(markup).toContain('data-paint-format="dir"');
		expect(markup).toContain('data-paint-format=".8f"');
		expect(markup).not.toContain('data-paint="forecast.forwardCurve.0"');
		expect(markup).not.toContain('data-paint="forecast.posterior.0.Scale"');
	});
});
