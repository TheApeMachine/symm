import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it } from "vitest";
import {
	candidatesAtom,
	phaseAtom,
	positionCountAtom,
	tickCountAtom,
} from "#/collections/app";
import { Pulse } from "#/components/pulse";

describe("Pulse", () => {
	beforeEach(() => {
		tickCountAtom.set(0);
		phaseAtom.set("—");
		candidatesAtom.set(0);
		positionCountAtom.set(0);
	});

	it("renders fallback placeholders initially", () => {
		const html = renderToStaticMarkup(<Pulse />);
		expect(html).toContain('data-read="tick"');
		expect(html).toContain('data-read="phase"');
		expect(html).toContain('data-read="cand"');
		expect(html).toContain('data-read="meas"');
		expect(html).toContain('data-read="open"');
	});

	it("renders updated atomic values", () => {
		tickCountAtom.set(1234);
		phaseAtom.set("RESONANCE");
		candidatesAtom.set(7);
		positionCountAtom.set(3);

		const html = renderToStaticMarkup(<Pulse />);
		expect(html).toContain("1234");
		expect(html).toContain("RESONANCE");
		expect(html).toContain("7");
		expect(html).toContain("3");
	});
});
