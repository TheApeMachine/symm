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

describe("the metric cut as the Workspace binds it", () => {
	it("groups all native metric coordinates into one gathering port", async () => {
		const systemGraph = (await import("../../../../manifest/system.json")).default;
		const bindings = JSON.parse(
			systemGraph.nodes.cut_consumer.inputData.bindings.value,
		) as { target: string }[];
		const wired = bindings
			.map((binding) => binding.target)
			.filter((target) => target.startsWith("gather.values"))
			.map((target) => target.slice("gather.".length));
		const groups = groupPorts(wired.map(port));

		expect(wired.length).toBeGreaterThan(400);
		expect(groups.map((group) => group.base)).toEqual(["values"]);
		expect(groups[0].members).toHaveLength(wired.length);
	});
});
