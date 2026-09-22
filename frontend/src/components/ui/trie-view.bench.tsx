import { renderToStaticMarkup } from "react-dom/server";
import { bench, describe } from "vitest";
import { TrieView, selectTriePath, type TrieNode } from "./trie-view";
// Fixture: 64 measured alternatives under one root, for layout and rendering.
const root: TrieNode = {
	id: "root",
	prefix: "root",
	probability: 1,
	children: Array.from({ length: 64 }, (_, index) => ({
		id: String(index),
		prefix: `branch ${index}`,
		probability: 1 / 64,
	})),
};
describe("TrieView", () => {
	bench("selects the highest supplied terminal probability", () => {
		selectTriePath(root);
	});
	bench("lays out and renders 65 supplied nodes", () => {
		renderToStaticMarkup(<TrieView root={root} />);
	});
});
