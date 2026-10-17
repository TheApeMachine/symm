// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { cognitionStore } from "#/collections/cognition";
import { Engine } from "#/components/engine";
import { SequiturGrammarViz } from "./sequitur-grammar-viz";

const problemTree = {
	epoch: "0",
	paths: 0,
	trading: false,
	gate: null,
	keys: [],
	branches: [],
	problem: {
		level: "error" as const,
		reason: "the newest stored policy is malformed: data: policy has no gate",
		fix: "re-run detect and train on the tick epoch",
	},
};

afterEach(() => {
	cleanup();
	cognitionStore.setState(() => ({ tree: null, matches: {} }));
});

describe("ModelProblem banner", () => {
	it("is absent while the tree is usable or not yet received", () => {
		const { container } = render(<Engine />);
		expect(container.querySelector("[data-problem]")).toBeNull();
	});

	it("cannot stretch the engine rail past its fixed width", () => {
		cognitionStore.setState(() => ({
			tree: {
				...problemTree,
				problem: {
					level: "error",
					reason:
						"policy epoch 1792282175840704000 stores no coder state (trained before coder state was saved); its gate would start cold, not where train left it",
					fix: "re-run detect and train on the tick epoch 1792169333015760400",
				},
			},
			matches: {},
		}));

		const { container } = render(<Engine />);
		const panel = container.firstElementChild;
		const banner = container.querySelector('[data-problem="error"]');

		expect(panel?.className).toContain("min-w-0");
		expect(banner?.className).toContain("min-w-0");
		expect(banner?.className).toContain("wrap-anywhere");
	});

	it("shows in the engine panel and the Sequitur tab", () => {
		cognitionStore.setState(() => ({ tree: problemTree, matches: {} }));

		for (const view of [<Engine key="e" />, <SequiturGrammarViz key="s" />]) {
			const { container, unmount } = render(view);
			const banner = container.querySelector('[data-problem="error"]');
			expect(banner?.textContent).toContain(problemTree.problem.reason);
			expect(banner?.textContent).toContain(`Fix: ${problemTree.problem.fix}`);
			unmount();
		}
	});
});
