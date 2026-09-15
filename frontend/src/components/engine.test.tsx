import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
	candidatesStore,
	onlineStore,
	phaseStore,
	positionCountStore,
	tickCountStore,
} from "#/collections/app";
import { Engine } from "#/components/engine";

describe("Engine", () => {
	it("renders the run readout rows bound to their own stores", () => {
		const markup = renderToStaticMarkup(<Engine />);

		expect(markup).toContain('data-e="seq"');
		expect(markup).toContain('data-e="phase"');
		expect(markup).toContain('data-e="cand"');
		expect(markup).toContain('data-e="open"');
		expect(markup).toContain("observations");
		expect(markup).toContain("phase");
	});

	it("reads observations, decisions and phase from the live telemetry stores", () => {
		tickCountStore.setState(100);
		candidatesStore.setState(20);
		positionCountStore.setState(3);
		phaseStore.setState("learning");
		onlineStore.setState("ONLINE");

		const markup = renderToStaticMarkup(<Engine />);
		expect(markup).toContain(">100<");
		expect(markup).toContain(">20<");
		expect(markup).toContain(">3<");
		expect(markup).toContain(">learning<");

		onlineStore.setState("OFFLINE");
		expect(renderToStaticMarkup(<Engine />)).toContain(">offline<");
	});
});
