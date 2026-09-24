// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { RecognitionView } from "./recognition-view";

afterEach(cleanup);
describe("RecognitionView", () => {
	it("distinguishes absence from measured zero and never generates activity", () => {
		const { container, rerender } = render(<RecognitionView />);
		expect(screen.getByText("No recorded activity")).toBeDefined();
		expect(container.textContent).not.toContain("128");
		expect(container.textContent).not.toContain("+2.4 bp");
		rerender(
			<RecognitionView
				metricMap={{ accuracy: 0, action: "0", input_count: 0 }}
				recentActivity={[
					{
						id: "receipt",
						time: "12:00",
						actionStr: "recorded EXIT",
						edgeStr: "-1.0 bp",
					},
				]}
			/>,
		);
		expect(screen.getByText("0.0%")).toBeDefined();
		expect(screen.getByText("WAIT")).toBeDefined();
		expect(screen.getByText("recorded EXIT")).toBeDefined();
		rerender(<RecognitionView />);
		expect(screen.queryByText("recorded EXIT")).toBeNull();
	});
});
