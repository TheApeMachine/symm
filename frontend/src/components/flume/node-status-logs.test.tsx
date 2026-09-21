import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { NodeLogs } from "./Node/NodeLogs";
import type { NodeLogEntry } from "./context";

describe("Node Status and Logging UI", () => {
	const sampleLogs: NodeLogEntry[] = [
		{ timestamp: 1700000000000, level: "info", message: "Node initialized" },
		{ timestamp: 1700000001000, level: "warn", message: "High latency detected" },
		{ timestamp: 1700000002000, level: "error", message: "Failed to connect to socket" },
	];

	it("renders nothing when isOpen is false", () => {
		const html = renderToStaticMarkup(
			<NodeLogs
				nodeId="node_1"
				label="Test Node"
				logs={sampleLogs}
				status="init"
				isOpen={false}
				onClose={() => {}}
			/>,
		);
		expect(html).toBe("");
	});

	it("renders inline log console with counts and entries when isOpen is true", () => {
		const html = renderToStaticMarkup(
			<NodeLogs
				nodeId="node_1"
				label="Test Node"
				logs={sampleLogs}
				status="busy"
				isOpen={true}
				onClose={() => {}}
			/>,
		);

		// Container with data attribute
		expect(html).toContain('data-flume-node-logs="node_1"');

		// Filter buttons with correct counts
		expect(html).toContain("All (3)");
		expect(html).toContain("Info (1)");
		expect(html).toContain("Warn (1)");
		expect(html).toContain("Err (1)");

		// Log messages
		expect(html).toContain("Node initialized");
		expect(html).toContain("High latency detected");
		expect(html).toContain("Failed to connect to socket");

		// Log level tags
		expect(html).toContain("info");
		expect(html).toContain("warn");
		expect(html).toContain("error");
	});

	it("renders empty state placeholder when node has no logs", () => {
		const html = renderToStaticMarkup(
			<NodeLogs
				nodeId="node_empty"
				label="Empty Node"
				logs={[]}
				status="ready"
				isOpen={true}
				onClose={() => {}}
			/>,
		);

		expect(html).toContain('data-flume-node-logs="node_empty"');
		expect(html).toContain("No logs recorded yet");
		expect(html).toContain("All (0)");
	});
});
