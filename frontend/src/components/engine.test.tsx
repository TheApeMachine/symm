import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { atoms, signals } from "#/collections/app";
import { Engine } from "#/components/engine";

describe("Engine", () => {
	it("renders the run readout rows bound to their own stores", () => {
		const markup = renderToStaticMarkup(<Engine />);

		expect(markup).toContain('data-e="seq"');
		expect(markup).toContain('data-e="phase"');
		expect(markup).toContain('data-e="decisions"');
		expect(markup).toContain('data-e="open"');
		expect(markup).toContain("observations");
		expect(markup).toContain("phase");
	});

	it("reads observations, decisions and phase from the live telemetry stores", () => {
		atoms.tick.set({ epoch: 0, n: 100 });
		signals.decisions.setState(
			Object.fromEntries(
				Array.from({ length: 20 }, (_, i) => [
					`SYM${i}/USD/virtual`,
					{
						symbol: `SYM${i}/USD`,
						action: "skip",
						tier: "virtual",
					},
				]),
			),
		);
		atoms.positionCount.set(3);
		atoms.phase.set("learning");
		atoms.online.set("ONLINE");

		const markup = renderToStaticMarkup(<Engine />);
		expect(markup).toContain(">100<");
		expect(markup).toContain(">20<");
		expect(markup).toContain(">3<");
		expect(markup).toContain(">learning<");

		atoms.online.set("OFFLINE");
		expect(renderToStaticMarkup(<Engine />)).toContain(">offline<");
	});
});
