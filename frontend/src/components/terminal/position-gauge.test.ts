import { describe, expect, it, vi } from "vitest";
import type { Holding, Position, Stoploss } from "#/collections/types";
import {
	createPositionGaugeElement,
	paintPositionHoldings,
	positionGaugeGeometry,
} from "./position-gauge";

const holding = (mark: number, returnPct: number): Holding => ({
	status: "open",
	symbol: "XLM/USD",
	asset: "XLM",
	qty: 10,
	sellable_qty: 10,
	entry_price: 100,
	entry_fee: 2.6,
	exit_price: 0,
	exit_fee: 2.6,
	mark,
	pnl: returnPct * 100,
	return_pct: returnPct,
	is_opportunity: false,
});

const stoploss = (): Stoploss => ({
	status: "armed",
	symbol: "XLM/USD",
	entry: 100,
	peak: 102,
	mark: 101.9,
	floor: 101,
});

const position = (mark: number, returnPct: number): Position => ({
	status: "open",
	entry_order: {},
	exit_order: {},
	order_id: "oid",
	fills: [],
	buffered: [],
	holding: {
		...holding(mark, returnPct),
		stoploss: stoploss(),
	},
});

describe("positionGaugeGeometry", () => {
	it("anchors the price gauge to entry, stop, peak, and mark returns", () => {
		const geometry = positionGaugeGeometry(holding(101.9, 0.019), stoploss());

		expect(geometry).not.toBeNull();
		expect(geometry?.stopPct).not.toBeNull();
		expect(geometry?.peakPct).not.toBeNull();
		expect(geometry?.stopPct).toBeGreaterThan(geometry?.entryPct ?? 0);
		expect(geometry?.markPct).toBeGreaterThan(geometry?.entryPct ?? 0);
		expect(geometry?.peakPct).toBeGreaterThan(geometry?.markPct ?? 0);
	});

	it("requires an explicit mark instead of deriving one from return_pct", () => {
		const geometry = positionGaugeGeometry(holding(0, 0.015), stoploss());

		expect(geometry).toBeNull();
	});

	it("scales mark distance from entry when no stop frame exists", () => {
		const geometry = positionGaugeGeometry(holding(95, -0.05));

		expect(geometry).not.toBeNull();
		expect(geometry?.entryPct).toBeGreaterThan(geometry?.markPct ?? 0);
		expect(geometry?.stopPct).toBeNull();
		expect(geometry?.peakPct).toBeNull();
	});
});

describe("paintPositionHoldings", () => {
	it("paints restored map-shaped holdings with decimal strings", () => {
		const elements: Array<Record<string, any>> = [];
		const element = () => {
			const node: Record<string, any> = {
				children: [],
				dataset: {},
				style: {},
				className: "",
				append: (...children: unknown[]) => node.children.push(...children),
				appendChild: (child: unknown) => node.children.push(child),
				addEventListener: () => undefined,
				setAttribute: () => undefined,
				querySelector: (selector: string) =>
					elements.find(
						(candidate) =>
							selector === `[data-gauge="${candidate.dataset.gauge}"]`,
						) ?? null,
			};
			let text = "";

			Object.defineProperty(node, "textContent", {
				get: () =>
					`${text}${node.children.map((child: any) => child.textContent ?? "").join("")}`,
				set: (value) => {
					text = String(value ?? "");
				},
			});
			elements.push(node);

			return node;
		};
		const body = element();
		body.replaceChildren = (...children: unknown[]) => {
			body.children = children;
		};

		vi.stubGlobal("document", {
			body,
			createElement: element,
		});

		document.body.replaceChildren(createPositionGaugeElement("EUL/USD", "USD"));

		paintPositionHoldings(
			{
				"EUL/USD": {
					status: "open",
					entry_order: {},
					exit_order: {},
					order_id: "oid",
					fills: [],
					buffered: [],
					holding: {
						symbol: "EUL/USD",
						asset: "EUL",
						qty: "9.85795428",
						sellable_qty: "9.85795428",
						entry_price: "1.729",
						entry_fee: "0.044315447670311994",
						exit_price: "0",
						exit_fee: "0",
						mark: "1.73",
						pnl: "0.00985795428",
						return_pct: "0.0005783337200618928",
						is_opportunity: false,
					},
				},
			},
			"BTC/USD",
		);

		expect(document.body.textContent).toContain("P/L 0.0099 USD");
		expect(document.body.textContent).toContain("entry 1.729 / mark 1.7300");
		vi.unstubAllGlobals();
	});

	it("paints nested stoploss fields from the holding payload directly", () => {
		const elements: Array<Record<string, any>> = [];
		const element = () => {
			const node: Record<string, any> = {
				children: [],
				dataset: {},
				style: {},
				className: "",
				append: (...children: unknown[]) => node.children.push(...children),
				appendChild: (child: unknown) => node.children.push(child),
				addEventListener: () => undefined,
				setAttribute: () => undefined,
				querySelector: (selector: string) =>
					elements.find(
						(candidate) =>
							selector === `[data-gauge="${candidate.dataset.gauge}"]`,
					) ?? null,
			};
			let text = "";

			Object.defineProperty(node, "textContent", {
				get: () =>
					`${text}${node.children.map((child: any) => child.textContent ?? "").join("")}`,
				set: (value) => {
					text = String(value ?? "");
				},
			});
			elements.push(node);

			return node;
		};
		const body = element();
		body.replaceChildren = (...children: unknown[]) => {
			body.children = children;
		};

		vi.stubGlobal("document", {
			body,
			createElement: element,
		});

		document.body.replaceChildren(createPositionGaugeElement("BABYSHARK/USD", "USD"));

		paintPositionHoldings(
			{
				"BABYSHARK/USD": {
					status: "open",
					entry_order: {},
					exit_order: {},
					order_id: "oid",
					fills: [],
					buffered: [],
					holding: {
						symbol: "BABYSHARK/USD",
						asset: "BABYSHARK",
						qty: "2151.4756",
						sellable_qty: "2151.4756",
						entry_price: "0.0742",
						entry_fee: "0",
						exit_price: "0",
						exit_fee: "0",
						mark: "0.0745",
						pnl: "0.6454",
						return_pct: "0.0040",
						is_opportunity: false,
						stoploss: {
							status: "armed",
							symbol: "BABYSHARK/USD",
							entry: "0.0742",
							peak: "0.0745",
							mark: "0.0745",
							floor: "0.0730",
						},
					},
				},
			},
			"USD",
		);

		expect(document.body.textContent).toContain("peak 0.0745");
		expect(document.body.textContent).toContain("floor 0.0730");
		vi.unstubAllGlobals();
	});

	it("renders backend zero economics literally instead of inventing fallback values", () => {
		const elements: Array<Record<string, any>> = [];
		const element = () => {
			const node: Record<string, any> = {
				children: [],
				dataset: {},
				style: {},
				className: "",
				append: (...children: unknown[]) => node.children.push(...children),
				appendChild: (child: unknown) => node.children.push(child),
				addEventListener: () => undefined,
				setAttribute: () => undefined,
				querySelector: (selector: string) =>
					elements.find(
						(candidate) =>
							selector === `[data-gauge="${candidate.dataset.gauge}"]`,
						) ?? null,
			};
			let text = "";

			Object.defineProperty(node, "textContent", {
				get: () =>
					`${text}${node.children.map((child: any) => child.textContent ?? "").join("")}`,
				set: (value) => {
					text = String(value ?? "");
				},
			});
			elements.push(node);

			return node;
		};
		const body = element();
		body.replaceChildren = (...children: unknown[]) => {
			body.children = children;
		};

		vi.stubGlobal("document", {
			body,
			createElement: element,
		});

		document.body.replaceChildren(createPositionGaugeElement("BABYSHARK/USD", "USD"));

		paintPositionHoldings(
			{
				"BABYSHARK/USD": {
					status: "open",
					entry_order: {},
					exit_order: {},
					order_id: "oid",
					fills: [],
					buffered: [],
					holding: {
						symbol: "BABYSHARK/USD",
						asset: "BABYSHARK",
						qty: "1.2",
						sellable_qty: "1.2",
						entry_price: "0",
						entry_fee: "0",
						exit_price: "0",
						exit_fee: "0",
						mark: "0",
						pnl: "0",
						return_pct: "0",
						is_opportunity: false,
					},
				},
			},
			"USD",
		);

		expect(document.body.textContent).toContain("P/L 0.0000 USD");
		expect(document.body.textContent).toContain("entry 0.000 / mark --");
		vi.unstubAllGlobals();
	});
});
