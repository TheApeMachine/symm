import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { XrayHawkesPanel } from "./xray-hawkes";

describe("XrayHawkesPanel", () => {
	it("renders the arrival-process readouts and canvas shell", () => {
		const markup = renderToStaticMarkup(<XrayHawkesPanel />);

		expect(markup).toContain('<canvas');
		expect(markup).toContain('data-f="events"');
		expect(markup).toContain('data-f="lambda"');
		expect(markup).toContain('data-f="mu"');
		expect(markup).toContain('data-f="sells"');
		expect(markup).toContain('data-f="eta"');
	});
});
