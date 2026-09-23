import { describe, expect, it } from "vitest";
import { groupPorts, portFamily } from "./port-families";
import type { PortType } from "./types";

const port = (name: string) => ({ name, label: name }) as PortType;

describe("portFamily", () => {
	it("reads the port a numbered slot belongs to", () => {
		expect(portFamily("metrics_7")).toBe("metrics");
		expect(portFamily("values_0")).toBe("values");
	});

	it("leaves a port that has no slots alone", () => {
		expect(portFamily("metrics")).toBeNull();
		expect(portFamily("buyQuantity.a")).toBeNull();
	});
});

describe("groupPorts", () => {
	it("gathers a port's slots back into the one port", () => {
		const groups = groupPorts([
			port("interests"),
			port("data"),
			port("data_1"),
			port("metrics"),
			port("metrics_1"),
			port("metrics_2"),
		]);

		expect(groups.map((group) => group.base)).toEqual([
			"interests",
			"data",
			"metrics",
		]);
		expect(groups[2].members).toHaveLength(3);
	});

	it("anchors a family on the port itself, not on a slot", () => {
		const [group] = groupPorts([port("metrics_1"), port("metrics")]);

		expect(group.representative.name).toBe("metrics");
	});

	it("keeps a port with no slots as a family of one", () => {
		const [group] = groupPorts([port("buyQuantity.a")]);

		expect(group.members).toHaveLength(1);
		expect(group.representative.name).toBe("buyQuantity.a");
	});
});

describe("the grid as the signals graph wires it", () => {
	/*
		The grid collects every metric in the system on one gathering port.
		Drawn a row per slot it is four hundred rows tall, which is what made
		the node unreadable.
	*/
	it("draws one row for a gathering port however many slots it has", async () => {
		const signalsGraph = (await import("../../../../manifest/signals.json"))
			.default as unknown as {
			nodes: Record<string, { connections?: { inputs?: Record<string, unknown> } }>;
		};

		const wired = Object.keys(signalsGraph.nodes.grid.connections?.inputs ?? {});
		const groups = groupPorts(wired.map(port));

		expect(wired.length).toBeGreaterThan(400);
		// Four hundred and some wired slots under the metrics port family.
		expect(groups.map((group) => group.base).sort()).toEqual([
			"metrics",
		]);

		const metrics = groups.find((group) => group.base === "metrics");
		expect(metrics?.members.length).toBeGreaterThan(400);
	});
});
