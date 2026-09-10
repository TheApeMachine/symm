// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";
import { LearningProgress } from "./charts";
import { learningFixture } from "./fixture";
import { projectLearning } from "./state";

describe("LearningProgress", () => {
	it("measures arrivals despite uninitialized and regressing producer clocks", async () => {
		const container = document.createElement("div");
		const root = createRoot(container);
		const now = vi.spyOn(performance, "now");
		const initial = projectLearning(learningFixture(), "");
		initial.forward.trained = 0;
		initial.at = "1970-01-01T00:00:00Z";

		try {
			now.mockReturnValue(1000);
			await act(async () => root.render(<LearningProgress view={initial} />));
			expect(container.textContent).toContain("measuring the rate");
			now.mockReturnValue(31000);
			const next = {
				...initial,
				at: "2026-09-10T00:00:00Z",
				forward: { ...initial.forward, trained: 1 },
			};
			await act(async () => root.render(<LearningProgress view={next} />));
			expect(container.textContent).toContain("2.0 per minute");
			now.mockReturnValue(61000);
			const regressed = {
				...next,
				at: "2026-09-09T00:00:00Z",
				forward: { ...next.forward, trained: 2 },
			};
			await act(async () => root.render(<LearningProgress view={regressed} />));
			expect(container.textContent).toContain("2.0 per minute");
		} finally {
			await act(async () => root.unmount());
			now.mockRestore();
		}
	});
});
