// @vitest-environment jsdom
import {
	cleanup,
	fireEvent,
	render,
	screen,
	waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { selectTriePath, type TrieNode, TrieView } from "./trie-view";

afterEach(cleanup);
describe("TrieView", () => {
	it("draws recorded actions on nodes and region tokens on edges, and replaces snapshots", async () => {
		const root: TrieNode = {
			id: "root",
			prefix: "",
			probability: 1,
			children: [
				{
					id: "a",
					prefix: "211",
					probability: 0.8,
					tokens: ["211"],
					label: "ENTER",
					share: 0.75,
				},
				{
					id: "b",
					prefix: "214",
					probability: 0.2,
					tokens: ["214"],
					label: "EXIT",
					share: 1,
				},
			],
		};
		const before = JSON.stringify(root);
		const { rerender } = render(<TrieView root={root} />);
		expect(screen.getByText("ROOT")).toBeDefined();
		expect(screen.getByText("ENTER")).toBeDefined();
		expect(screen.getByText("[211]")).toBeDefined();
		expect(screen.getByText("75%")).toBeDefined();
		fireEvent.click(screen.getByText("HIGHEST PROBABILITY PATH"));
		await waitFor(() => expect(screen.queryByText("EXIT")).toBeNull());
		fireEvent.click(screen.getByText("ALL PATHS"));
		expect(screen.getByText("EXIT")).toBeDefined();
		fireEvent.keyDown(screen.getByRole("button", { name: "Toggle root" }), {
			key: "Enter",
		});
		await waitFor(() => expect(screen.queryByText("ENTER")).toBeNull());
		expect(JSON.stringify(root)).toBe(before);
		rerender(
			<TrieView
				root={{
					id: "new",
					prefix: "",
					probability: 1,
					children: [{ id: "w", prefix: "290", probability: 1, label: "WAIT" }],
				}}
			/>,
		);
		expect(screen.getByText("WAIT")).toBeDefined();
		rerender(<TrieView />);
		expect(screen.getByText("No recorded trie")).toBeDefined();
		await waitFor(() => expect(screen.queryByText("WAIT")).toBeNull());
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
