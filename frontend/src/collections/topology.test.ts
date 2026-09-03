import { describe, expect, it } from "vitest";
import type { EnvelopeBoundaryStamp } from "#/providers/telemetry/telemetry/envelope-boundary-stamp";
import { topologyStore } from "./topology";

/*
Stamp is the slice of EnvelopeBoundaryStamp that ingest actually reads. The
generated flatbuffer accessor carries far more surface than the store touches,
so the fakes implement exactly what is used and are narrowed to it.
*/
type Stamp = Pick<
	EnvelopeBoundaryStamp,
	| "label"
	| "group"
	| "stage"
	| "seqCount"
	| "avgGapNs"
	| "lastGapNs"
	| "atNs"
	| "backlog"
>;

/*
stamp builds one boundary stamp the way the runtime emits it: a label, the
ring it ran in, and its handler-group index within that ring.
*/
const stamp = (
	label: string,
	group: string,
	stage: number,
	atNs: number,
): Stamp => ({
	label: () => label,
	group: () => group,
	stage: () => stage,
	seqCount: () => 1n,
	avgGapNs: () => 1_000n,
	lastGapNs: () => 1_000n,
	atNs: () => BigInt(atNs),
	backlog: () => 0n,
});

const ingest = (stamps: Stamp[]) => {
	topologyStore.actions.ingest(stamps as EnvelopeBoundaryStamp[]);
};

const edgeKeys = () => [...topologyStore.state.edges.keys()].sort();

describe("topologyStore.ingest", () => {
	it("never wires two stages of the same handler group together", () => {
		topologyStore.setState(() => ({
			nodes: new Map(),
			edges: new Map(),
			version: 0,
		}));

		// Three advisors race inside one group. Each envelope observes them in
		// a different completion order, which is exactly what turned the graph
		// into a mesh: pairing consecutive stamps mints an edge per ordering.
		ingest([
			stamp("ingress", "ring", 0, 0),
			stamp("auction", "advisors", 1, 10),
			stamp("basis", "advisors", 1, 20),
			stamp("momentum", "advisors", 1, 30),
			stamp("planned", "ring", 2, 40),
		]);

		ingest([
			stamp("ingress", "ring", 0, 100),
			stamp("momentum", "advisors", 1, 110),
			stamp("auction", "advisors", 1, 120),
			stamp("basis", "advisors", 1, 130),
			stamp("planned", "ring", 2, 140),
		]);

		const keys = edgeKeys();

		for (const sibling of ["auction", "basis", "momentum"]) {
			for (const other of ["auction", "basis", "momentum"]) {
				expect(keys).not.toContain(`${sibling}>${other}`);
			}
		}
	});

	it("fans the whole group in and out of its neighbours", () => {
		topologyStore.setState(() => ({
			nodes: new Map(),
			edges: new Map(),
			version: 0,
		}));

		ingest([
			stamp("ingress", "ring", 0, 0),
			stamp("auction", "advisors", 1, 10),
			stamp("basis", "advisors", 1, 20),
			stamp("planned", "ring", 2, 30),
		]);

		expect(edgeKeys()).toEqual([
			"auction>planned",
			"basis>planned",
			"ingress>auction",
			"ingress>basis",
		]);
	});

	it("keeps the edge count linear in stages rather than quadratic", () => {
		topologyStore.setState(() => ({
			nodes: new Map(),
			edges: new Map(),
			version: 0,
		}));

		const siblings = ["a", "b", "c", "d", "e", "f", "g"];

		// Every permutation the race could produce, many times over.
		for (let round = 0; round < 20; round++) {
			const order = [...siblings].sort(() => Math.random() - 0.5);
			ingest([
				stamp("ingress", "ring", 0, round * 1000),
				...order.map((label, index) =>
					stamp(label, "advisors", 1, round * 1000 + index + 1),
				),
				stamp("planned", "ring", 2, round * 1000 + 100),
			]);
		}

		// Exactly the fan-in and fan-out: 7 in, 7 out. A sibling mesh would
		// add up to 7*6 = 42 more.
		expect(topologyStore.state.edges.size).toBe(siblings.length * 2);
	});

	it("still records a real hop between distinct groups", () => {
		topologyStore.setState(() => ({
			nodes: new Map(),
			edges: new Map(),
			version: 0,
		}));

		ingest([stamp("signal", "ring", 0, 0), stamp("category", "logic", 1, 500)]);

		const hop = topologyStore.state.edges.get("signal>category");

		expect(hop).toBeDefined();
		expect(hop?.hopCount).toBe(1);
		expect(hop?.lastLatencyNs).toBe(500);
	});
});
