// @vitest-environment jsdom

import { act, render } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import Connections from "./Connections/Connections";
import { CONNECTIONS_ID } from "./constants";
import { flumeEditorStore, setSelectedNode } from "./flume-editor.store";

describe("Node and Edge Selection", () => {
	const editorId = "editor_test_1";

	beforeEach(() => {
		document.body.innerHTML = "";
		setSelectedNode(editorId, null);
	});

	it("manages selected node state per editorId without collision", () => {
		setSelectedNode(editorId, "node_alpha");
		setSelectedNode("editor_test_2", "node_beta");

		expect(flumeEditorStore.state.selectedNodeIdByEditorId[editorId]).toBe(
			"node_alpha",
		);
		expect(flumeEditorStore.state.selectedNodeIdByEditorId.editor_test_2).toBe(
			"node_beta",
		);

		setSelectedNode(editorId, null);
		expect(
			flumeEditorStore.state.selectedNodeIdByEditorId[editorId],
		).toBeUndefined();
		expect(flumeEditorStore.state.selectedNodeIdByEditorId.editor_test_2).toBe(
			"node_beta",
		);
	});

	it("highlights incident edges and dims non-incident edges when node is selected", () => {
		const { container } = render(<Connections editorId={editorId} />);
		const connectionsContainer = container.querySelector(
			`#${CONNECTIONS_ID}${editorId}`,
		);
		expect(connectionsContainer).not.toBeNull();

		// Create mock SVG connection paths:
		// edge 1: node_alpha -> node_beta
		// edge 2: node_gamma -> node_alpha
		// edge 3: node_beta -> node_delta (unrelated)
		const createMockPath = (
			id: string,
			outputNode: string,
			inputNode: string,
		) => {
			const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
			const path = document.createElementNS(
				"http://www.w3.org/2000/svg",
				"path",
			);
			path.setAttribute("data-connection-id", id);
			path.setAttribute("data-output-node-id", outputNode);
			path.setAttribute("data-input-node-id", inputNode);
			svg.appendChild(path);
			connectionsContainer?.appendChild(svg);
			return path;
		};

		const edgeOne = createMockPath("edge_1", "node_alpha", "node_beta");
		const edgeTwo = createMockPath("edge_2", "node_gamma", "node_alpha");
		const edgeThree = createMockPath("edge_3", "node_beta", "node_delta");

		// Select node_alpha
		act(() => {
			setSelectedNode(editorId, "node_alpha");
		});

		expect(connectionsContainer?.getAttribute("data-has-selection")).toBe(
			"true",
		);
		expect(edgeOne.getAttribute("data-highlighted")).toBe("true");
		expect(edgeOne.parentElement?.style.zIndex).toBe("10");

		expect(edgeTwo.getAttribute("data-highlighted")).toBe("true");
		expect(edgeTwo.parentElement?.style.zIndex).toBe("10");

		expect(edgeThree.getAttribute("data-highlighted")).toBeNull();
		expect(edgeThree.parentElement?.style.zIndex).toBe("0");

		// Deselect node
		act(() => {
			setSelectedNode(editorId, null);
		});

		expect(connectionsContainer?.getAttribute("data-has-selection")).toBeNull();
		expect(edgeOne.getAttribute("data-highlighted")).toBeNull();
		expect(edgeOne.parentElement?.style.zIndex).toBe("0");
		expect(edgeTwo.getAttribute("data-highlighted")).toBeNull();
		expect(edgeTwo.parentElement?.style.zIndex).toBe("0");
		expect(edgeThree.getAttribute("data-highlighted")).toBeNull();
	});
});
