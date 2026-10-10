// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import {
	formatValue,
	memoizedQuery,
	memoizedQueryAll,
	renderValue,
	resetValue,
	setText,
} from "./dom";

describe("dom utilities", () => {
	describe("memoizedQuery and memoizedQueryAll", () => {
		it("caches query results per element and selector", () => {
			const container = document.createElement("div");
			container.innerHTML = `
				<div class="header">Header</div>
				<div class="item">1</div>
				<div class="item">2</div>
			`;

			const first = memoizedQuery(container, ".header");
			expect(first?.textContent).toBe("Header");

			// Repeated call retrieves cached reference
			const cached = memoizedQuery(container, ".header");
			expect(cached).toBe(first);

			const all = memoizedQueryAll(container, ".item");
			expect(all.length).toBe(2);
			const cachedAll = memoizedQueryAll(container, ".item");
			expect(cachedAll).toBe(all);
		});
	});

	describe("setText", () => {
		it("sets textContent and innerText", () => {
			const el = document.createElement("span");
			setText(el, "test-value");
			expect(el.textContent).toBe("test-value");
			expect(el.innerText).toBe("test-value");
		});
	});

	describe("formatValue", () => {
		it("formats percentage correctly", () => {
			expect(formatValue(0.425, "pct")).toBe("42.5%");
		});

		it("formats basis points correctly", () => {
			expect(formatValue(0.00124, "bp")).toBe("12.4 bp");
		});

		it("formats integer with separators", () => {
			expect(formatValue(1234567, "int")).toBe("1,234,567");
		});

		it("formats currency/eur", () => {
			expect(formatValue(14.5, "eur")).toBe("€14.50");
			expect(formatValue(14.5, "currency")).toBe("€14.50");
		});

		it("formats pnl with sign", () => {
			expect(formatValue(1.2345, "pnl")).toBe("+€1.2345");
			expect(formatValue(-1.2345, "pnl")).toBe("-€1.2345");
		});

		it("formats bits, nat, and spread", () => {
			expect(formatValue(1.85, "bits")).toBe("1.85 bits");
			expect(formatValue(2.4, "nat")).toBe("2.40 nat");
			expect(formatValue(0.05, "spread")).toBe("Spread 0.050");
		});

		it("formats fixed precision dynamically", () => {
			expect(formatValue(12.34567, "fixed:2")).toBe("12.35");
			expect(formatValue(12.34567, "fixed:0")).toBe("12");
			expect(formatValue(12.34567, "fixed:5")).toBe("12.34567");
		});

		it("defaults to 4 decimals when format is empty or unspecified", () => {
			expect(formatValue(12.34567)).toBe("12.3457");
			expect(formatValue(0)).toBe("0.0000");
		});
	});

	describe("renderValue and resetValue", () => {
		it("renders numerical values adhering to data-format attribute", () => {
			const el = document.createElement("span");
			el.dataset.format = "pct";
			renderValue(el, 0.755);
			expect(el.textContent).toBe("75.5%");

			el.dataset.format = "fixed:1";
			renderValue(el, 9.87);
			expect(el.textContent).toBe("9.9");
		});

		it("preserves valid value when receiving NaN or undefined once seen", () => {
			const el = document.createElement("span");
			renderValue(el, 42.5);
			expect(el.textContent).toBe("42.5000");

			// Transient NaN must not wipe valid value
			renderValue(el, Number.NaN);
			expect(el.textContent).toBe("42.5000");

			// Transient undefined must not wipe valid value once seen
			renderValue(el, undefined);
			expect(el.textContent).toBe("42.5000");
		});

		it("renders strings and booleans directly", () => {
			const strEl = document.createElement("span");
			renderValue(strEl, "ENTER");
			expect(strEl.textContent).toBe("ENTER");

			const boolEl = document.createElement("span");
			renderValue(boolEl, true);
			expect(boolEl.textContent).toBe("TRUE");
			renderValue(boolEl, false);
			expect(boolEl.textContent).toBe("FALSE");
		});

		it("resets value on standby", () => {
			const el = document.createElement("span");
			renderValue(el, 123.456);
			expect(el.textContent).toBe("123.4560");

			resetValue(el);
			expect(el.textContent).toBe("--");

			el.dataset.fallback = "N/A";
			resetValue(el);
			expect(el.textContent).toBe("N/A");
		});
	});
});
