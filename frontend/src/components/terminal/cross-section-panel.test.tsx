// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { focusAtom, signals } from "#/collections/app";
import { Value } from "#/types/value";
import { CrossSectionPanel } from "./cross-section-panel";

const publishLiquidity = (metrics: Record<string, number>) => {
	const prefix = "workspace/tick/aggregate/0/1";
	const keys = Object.keys(metrics);
	const aggregate = new Value({
		origin: "workspace",
		role: "tick",
		scope: "aggregate",
		prefix,
		timestamp: 1_700_000_000_000_000_000n,
		index: keys.map((key) => `${prefix}/liquidity/BTC/USD/${key}`),
		layers: keys.map((key) =>
			new TextEncoder().encode(JSON.stringify([metrics[key]])),
		),
	});
	act(() => {
		signals.workspace.setState((prev) => ({ ...prev, aggregate }));
	});
};

afterEach(() => {
	cleanup();
	focusAtom.set("BTC/USD");
	signals.workspace.setState(() => ({}));
});

describe("CrossSectionPanel", () => {
	it("renders panel and binds liquidity metrics live", () => {
		const view = render(<CrossSectionPanel />);

		expect(
			view.container.querySelector('[data-metric="symbol"]')?.textContent,
		).toBe("—");

		publishLiquidity({
			relative_spread: 0.0004,
			"best_price:bid": 95123.45,
			"best_price:ask": 95127.25,
		});

		expect(
			view.container.querySelector('[data-metric="symbol"]')?.textContent,
		).toBe("BTC/USD");
		expect(
			view.container.querySelector('[data-metric-raw="best_price:bid"]')
				?.textContent,
		).toBe("95123.45");
		expect(
			view.container.querySelector('[data-metric-raw="best_price:ask"]')
				?.textContent,
		).toBe("95127.25");
		expect(
			view.container.querySelector('[data-metric-raw="relative_spread"]')
				?.textContent,
		).toBe("4.0 bp");
	});
});
