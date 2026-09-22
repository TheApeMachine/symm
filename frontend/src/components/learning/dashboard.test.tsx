// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { focusAtom, RingBuffer, signals } from "#/collections/app";
import type { WireMeasurement } from "#/types/capnp/measurement";
import { LearningDashboard } from "./dashboard";

afterEach(() => {
	cleanup();
	signals.training.setState(() => ({}));
});

describe("LearningDashboard", () => {
	it("reports unavailable grid data while rendering measured scalar metrics", () => {
		focusAtom.set("BTC/USD");
		const measurement: WireMeasurement = {
			id: "meas-learn-1",
			source: "training",
			symbol: "BTC/USD",
			tick: 1n,
			at: 1000n,
			timestamp: 1000n,
			entity: 1,
			maturity: 0.9,
			snr: 1,
			separation: 0,
			metrics: [{ name: "action", raw: 1, normalized: 1 }],
			metadata: {},
			provenance: [],
		};
		const ring = new RingBuffer<WireMeasurement>(4);
		ring.add(measurement);
		signals.training.setState(() => ({ "BTC/USD": ring }));
		const { container } = render(<LearningDashboard />);
		expect(
			container.querySelector<HTMLElement>('[data-metric="action"]')?.innerText,
		).toEqual("ENTER");
		fireEvent.click(screen.getByText("Impulse map"));
		expect(screen.getByText("Impulse map unavailable")).toBeDefined();
		expect(container.querySelector("canvas")).toBeNull();
	});
});
