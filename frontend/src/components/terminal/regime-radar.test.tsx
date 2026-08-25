import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { RadarPanel } from "./regime-radar";

describe("RadarPanel", () => {
	it("renders one arm per regime axis", () => {
		const markup = renderToStaticMarkup(<RadarPanel />);
		const arms = [...markup.matchAll(/data-axis="([^"]+)"/g)].map((m) => m[1]);

		expect(arms).toHaveLength(5);
		expect(arms).toEqual(["volatility", "trend", "drive", "starved", "chop"]);
	});
});
