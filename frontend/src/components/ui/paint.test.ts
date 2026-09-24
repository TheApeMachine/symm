// @vitest-environment jsdom

import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { applyPaintMap, type SubscribableStore, usePaintStore } from "./paint";

describe("paint system", () => {
	it("applyPaintMap updates fields, meters, badges, and variables directly", () => {
		const root = document.createElement("div");
		root.innerHTML = `
			<span data-f="winner">—</span>
			<span data-f="confidence">—</span>
			<div data-meter="depth" role="progressbar"><div data-meter-fill style="width: 0%"></div></div>
			<span data-badge="status" class="badge">OLD</span>
			<div data-axis="volatility"></div>
		`;

		applyPaintMap(root, {
			fields: {
				winner: "CLASSIFIED",
				confidence: "87.5%",
			},
			meters: {
				depth: { percent: 65, variant: "success" },
			},
			badges: {
				status: { label: "HEALTHY", variant: "success", size: "s" },
			},
			vars: {
				volatility: "0.85",
			},
		});

		expect(root.querySelector('[data-f="winner"]')?.textContent).toBe(
			"CLASSIFIED",
		);
		expect(root.querySelector('[data-f="confidence"]')?.textContent).toBe(
			"87.5%",
		);
		expect(
			(root.querySelector("[data-meter-fill]") as HTMLElement)?.style.width,
		).toBe("65%");
		expect(root.querySelector('[data-badge="status"]')?.textContent).toBe(
			"HEALTHY",
		);
		expect(
			(
				root.querySelector('[data-axis="volatility"]') as HTMLElement
			)?.style.getPropertyValue("--axis"),
		).toBe("0.85");
	});

	it("usePaintStore subscribes and cleans up on unmount", () => {
		type TestState = { count: number; name: string };
		let listener: ((state: TestState) => void) | null = null;
		const unsubscribeSpy = vi.fn();

		const store: SubscribableStore<TestState> = {
			state: { count: 1, name: "ALPHA" },
			subscribe: (fn) => {
				listener = fn;
				return { unsubscribe: unsubscribeSpy };
			},
		};

		const root = document.createElement("div");
		const field = document.createElement("span");
		field.setAttribute("data-f", "name");
		root.appendChild(field);

		const { unmount } = renderHook(() => {
			const ref = usePaintStore(store, (state) => ({
				fields: { name: state.name },
			}));
			// Attach simulated DOM element
			(ref as any).current = root;
		});

		// Trigger store update
		if (listener) {
			(listener as (s: TestState) => void)({ count: 2, name: "BETA" });
		}
		expect(field.textContent).toBe("BETA");

		unmount();
		expect(unsubscribeSpy).toHaveBeenCalled();
	});
});
