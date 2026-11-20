import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it } from "vitest";
import { atoms } from "#/collections/app";
import { TerminalTopBar } from "./terminal-top-bar";

describe("TerminalTopBar", () => {
	beforeEach(() => {
		atoms.tick.set(null);
	});

	it("renders the tick readout", () => {
		const html = renderToStaticMarkup(<TerminalTopBar />);
		expect(html).not.toContain("Observations");
		expect(html).toContain("Ticks");
		expect(html).not.toContain('data-observations="true"');
		expect(html).toContain('data-tick="true"');
	});

	it("renders updated tick count", () => {
		atoms.tick.set({ epoch: 0, n: 45678 });
		const html = renderToStaticMarkup(<TerminalTopBar />);
		expect(html).toContain("45,678");
	});
});
