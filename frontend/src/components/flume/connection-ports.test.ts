// @vitest-environment jsdom

import { describe, expect, it } from "vitest";
import { findPortHandle } from "./connection-ports";

const handle = (nodeId: string, portName: string, transput: string) => {
	const element = document.createElement("div");
	element.setAttribute("data-flume-component", "port-handle");
	element.setAttribute("data-node-id", nodeId);
	element.setAttribute("data-port-name", portName);
	element.setAttribute("data-port-transput-type", transput);
	document.body.appendChild(element);
	return element;
};

describe("findPortHandle", () => {
	it("finds a port drawn under its own name", () => {
		document.body.innerHTML = "";
		const port = handle("grid", "metrics_7", "input");

		expect(findPortHandle(document, "grid", "metrics_7", "input")).toBe(port);
	});

	it("anchors a slot on the collapsed port it belongs to", () => {
		document.body.innerHTML = "";
		const port = handle("grid", "metrics", "input");

		expect(findPortHandle(document, "grid", "metrics_7", "input")).toBe(port);
	});

	it("anchors a slot the node never declared as its own port", () => {
		document.body.innerHTML = "";
		// store.Grid declares one `values` output; values_5 exists only as
		// the name a consumer wired to.
		const port = handle("grid", "values", "output");

		expect(findPortHandle(document, "grid", "values_5", "output")).toBe(port);
	});

	it("reports nothing when neither the slot nor its port is drawn", () => {
		document.body.innerHTML = "";

		expect(findPortHandle(document, "grid", "metrics_7", "input")).toBeNull();
	});
});
