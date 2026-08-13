import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { RadarPanel } from "./regime-radar";

describe("RadarPanel", () => {
	it("paints each normalized reading on the group whose scale it controls", () => {
		const markup = renderToStaticMarkup(<RadarPanel />);
		const arms = [...markup.matchAll(/<g[^>]*data-set="([^"]+)"[^>]*>/g)];

		expect(arms).toHaveLength(5);
		expect(arms.map((match) => match[1])).toEqual([
			"metrics.spectral_radius.normalized",
			"metrics.trend.normalized",
			"metrics.drive.normalized",
			"metrics.starvation.normalized",
			"metrics.balance.normalized",
		]);
		expect(markup.match(/data-target="style.--axis"/g)).toHaveLength(5);
		expect(markup).not.toMatch(/<line[^>]*data-set=/);
	});
});
