import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { XrayManifoldPanel } from "./xray-manifold-panel";

describe("XrayManifoldPanel", () => {
	it("renders manifold rows and continuous dynamics", () => {
		const markup = renderToStaticMarkup(<XrayManifoldPanel />);

		expect(markup).toContain("energy");
		expect(markup).toContain("surprise");
		expect(markup).toContain("base alpha");
		expect(markup).toContain("Continuous dynamics");
		expect(markup).toContain("velocity");
		expect(markup).toContain("rotor norm");
		expect(markup).toContain('data-f="d0"');
	});
});
