// @vitest-environment jsdom
import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { positionCountAtom, signals } from "#/collections/app";
import { LearningDashboard, tabAtom } from "./dashboard";

afterEach(() => {
	cleanup();
	signals.training.setState(() => ({}));
	positionCountAtom.set(0);
	tabAtom.set("forward");
});

describe("LearningDashboard", () => {
	it("opens on the position life-cycle view by default", () => {
		const { container } = render(<LearningDashboard />);

		expect(container.textContent).toContain("Position life-cycle");
		expect(
			container.querySelector('[data-l="lifecycle-performance"]'),
		).not.toBeNull();
	});

	it("renders all learning dashboard navigation tabs", () => {
		const { getByText } = render(<LearningDashboard />);

		expect(getByText("Open positions")).toBeTruthy();
		expect(getByText("Position life-cycle")).toBeTruthy();
		expect(getByText("Historical runs")).toBeTruthy();
		expect(getByText("Cognitive tree")).toBeTruthy();
		expect(getByText("Impulse map")).toBeTruthy();
	});

	it("switches to open positions view when tab is selected", () => {
		const { container, getByText } = render(<LearningDashboard />);

		act(() => {
			fireEvent.click(getByText("Open positions"));
		});

		expect(tabAtom.get()).toBe("positions");
		expect(container.textContent).toContain("No open positions");
	});

	it("switches to historical runs view when tab is selected", () => {
		const { container, getByText } = render(<LearningDashboard />);

		act(() => {
			fireEvent.click(getByText("Historical runs"));
		});

		expect(tabAtom.get()).toBe("historical");
		expect(container.textContent).toContain("HISTORICAL RUNS");
	});

	it("switches to cognitive tree view when tab is selected", () => {
		const { container, getByText } = render(<LearningDashboard />);

		act(() => {
			fireEvent.click(getByText("Cognitive tree"));
		});

		expect(tabAtom.get()).toBe("cognitive");
		expect(container.textContent).toContain("Loading stored paths…");
	});

	it("switches to impulse map view when tab is selected", () => {
		const { container, getByText } = render(<LearningDashboard />);

		act(() => {
			fireEvent.click(getByText("Impulse map"));
		});

		expect(tabAtom.get()).toBe("impulse");
		expect(container.textContent).toContain("Quiescent · Awaiting impulse");
	});

	it("switches to sequitur grammar view when tab is selected", () => {
		const { container, getByText } = render(<LearningDashboard />);

		act(() => {
			fireEvent.click(getByText("Sequitur grammar"));
		});

		expect(tabAtom.get()).toBe("sequitur");
		expect(container.textContent).toContain("Sequitur Grammar");
	});
});
