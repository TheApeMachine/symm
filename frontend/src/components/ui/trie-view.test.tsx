// @vitest-environment jsdom
import {
	cleanup,
	fireEvent,
	render,
	screen,
	waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TrieView, selectTriePath, type TrieNode } from "./trie-view";
afterEach(cleanup);
describe("TrieView", () => {
	it("uses supplied tree data for collapse and focus, and replaces snapshots", async () => {
		const root: TrieNode = {
			id: "root",
			prefix: "Actual root",
			probability: 1,
			children: [
				{ id: "a", prefix: "Observed A", probability: 0.8 },
				{ id: "b", prefix: "Observed B", probability: 0.2 },
			],
		};
		const before = JSON.stringify(root);
		const { rerender } = render(<TrieView root={root} />);
		expect(screen.getByText("Observed A")).toBeDefined();
		fireEvent.click(screen.getByText("HIGHEST PROBABILITY PATH"));
		await waitFor(() => expect(screen.queryByText("Observed B")).toBeNull());
		fireEvent.click(screen.getByText("ALL PATHS"));
		expect(screen.getByText("Observed B")).toBeDefined();
		fireEvent.keyDown(
			screen.getByRole("button", { name: "Toggle Actual root" }),
			{ key: "Enter" },
		);
		await waitFor(() => expect(screen.queryByText("Observed A")).toBeNull());
		expect(JSON.stringify(root)).toBe(before);
		rerender(
			<TrieView root={{ id: "new", prefix: "New snapshot", probability: 1 }} />,
		);
		expect(screen.getByText("New snapshot")).toBeDefined();
		rerender(<TrieView />);
		expect(screen.getByText("No recorded trie")).toBeDefined();
		await waitFor(() => expect(screen.queryByText("New snapshot")).toBeNull());
	});
});

describe("selectTriePath", () => {
	it("finds the strongest terminal path instead of greedily choosing the strongest parent", () => {
		const root: TrieNode = {
			id: "root",
			prefix: "root",
			probability: 1,
			children: [
				{
					id: "wide",
					prefix: "wide",
					probability: 0.8,
					children: [{ id: "weak", prefix: "weak", probability: 0.1 }],
				},
				{
					id: "narrow",
					prefix: "narrow",
					probability: 0.2,
					children: [{ id: "strong", prefix: "strong", probability: 0.2 }],
				},
			],
		};
		expect([...selectTriePath(root)]).toEqual(["root", "narrow", "strong"]);
		root.children![0].isEnd = true;
		expect([...selectTriePath(root)]).toEqual(["root", "wide"]);
	});
});
