/** @vitest-environment jsdom */

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TabbedChart } from "#/components/charts/tabbed";

describe("TabbedChart", () => {
	it("reports active tab changes", () => {
		const onActiveTabChange = vi.fn();

		render(
			<TabbedChart
				onActiveTabChange={onActiveTabChange}
				tabs={[
					{
						label: "First",
						icon: <span>first-icon</span>,
						component: <div>first-panel</div>,
					},
					{
						label: "Second",
						icon: <span>second-icon</span>,
						component: <div>second-panel</div>,
					},
				]}
			/>,
		);

		expect(onActiveTabChange).toHaveBeenCalledWith("First");

		fireEvent.click(screen.getByRole("tab", { name: /second-icon/i }));

		expect(onActiveTabChange).toHaveBeenCalledWith("Second");
	});

	it("applies a className to the root tabs container", () => {
		const { container } = render(
			<TabbedChart
				className="hidden"
				tabs={[
					{
						label: "Only",
						icon: <span>only-icon</span>,
						component: <div>only-panel</div>,
					},
				]}
			/>,
		);

		expect(
			(container.firstChild as HTMLElement).classList.contains("hidden"),
		).toBe(true);
	});
});
