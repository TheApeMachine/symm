import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { CognitionBranchT } from "#/providers/telemetry/telemetry/cognition-branch";
import { LearningLearnerT } from "#/providers/telemetry/telemetry/learning-learner";
import { TrieModal } from "./trie-modal";

const learner = (branches: number) => {
	const held = new LearningLearnerT();
	Object.assign(held, {
		id: 2,
		links: 110,
		branches: Array.from({ length: branches }, (_, index) =>
			Object.assign(new CognitionBranchT(), {
				id: BigInt(index),
				parentId: BigInt(index === 0 ? -1 : 0),
				token: index === 0 ? "•" : "enter",
				prefix: index === 0 ? "" : "enter",
				key: index === 0 ? "" : "enter",
				depth: BigInt(index === 0 ? 0 : 1),
				probability: 0.5,
				count: BigInt(4),
			}),
		),
	});

	return held;
};

describe("TrieModal", () => {
	it("names the learner and what it holds", () => {
		const markup = renderToStaticMarkup(
			<TrieModal learner={learner(3)} open onClose={() => {}} />,
		);

		// Learner ids are zero-based; the surface counts workers from one.
		expect(markup).toContain("Learner 3");
		expect(markup).toContain("110 learned situations");
		expect(markup).toContain("3 nodes drawn");
	});

	/*
		A learner that has committed nothing is not a learner with an empty
		tree: saying so is the reading, and drawing an empty canvas is not.
	*/
	it("says so when a learner holds nothing, rather than drawing nothing", () => {
		const markup = renderToStaticMarkup(
			<TrieModal learner={learner(0)} open onClose={() => {}} />,
		);

		expect(markup).toContain("has not committed anything to memory yet");
		expect(markup).not.toContain("<canvas");
	});
});
