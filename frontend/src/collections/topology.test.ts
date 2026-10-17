import { describe, expect, it } from "vitest";
import { type BoundaryStamp, topologyStore } from "./topology";

/*
Stamp is the slice of BoundaryStamp that ingest actually reads. The
generated flatbuffer accessor carries far more surface than the store touches,
so the fakes implement exactly what is used and are narrowed to it.
*/
type Stamp = Pick<
	BoundaryStamp,
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
	topologyStore.actions.ingest(stamps as BoundaryStamp[]);
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

	it("ingests Value objects and derives topology, stages, and hops", () => {
		topologyStore.setState(() => ({
			nodes: new Map(),
			edges: new Map(),
			version: 0,
		}));

		topologyStore.actions.ingestValues([
			{
				source: "kraken:trade",
				at: 1_000_000_000n,
			},
			{
				source: "cvd",
				at: 1_000_020_000n,
				peers: [{ source: "kraken", at: 1_000_000_000n }],
			},
			{
				source: "depthflow",
				at: 1_000_025_000n,
				peers: [{ source: "kraken", at: 1_000_000_000n }],
			},
			{
				source: "manifold",
				at: 1_000_050_000n,
				peers: [
					{ source: "cvd", at: 1_000_020_000n },
					{ source: "depthflow", at: 1_000_025_000n },
				],
			},
			{
				source: "training",
				at: 1_000_080_000n,
				peers: [{ source: "manifold", at: 1_000_050_000n }],
			},
		]);

		const { nodes, edges } = topologyStore.state;

		expect(nodes.has("kraken")).toBe(true);
		expect(nodes.has("cvd")).toBe(true);
		expect(nodes.has("depthflow")).toBe(true);
		expect(nodes.has("manifold")).toBe(true);
		expect(nodes.has("training")).toBe(true);

		expect(nodes.get("kraken")?.group).toBe("venue");
		expect(nodes.get("cvd")?.group).toBe("signal");
		expect(nodes.get("manifold")?.group).toBe("logic");
		expect(nodes.get("training")?.group).toBe("strategy");

		expect(edges.has("kraken>cvd")).toBe(true);
		expect(edges.has("kraken>depthflow")).toBe(true);
		expect(edges.has("cvd>manifold")).toBe(true);
		expect(edges.has("manifold>training")).toBe(true);
		// Sibling signals do not hop each other
		expect(edges.has("cvd>depthflow")).toBe(false);
	});

	it("does not treat a tick timestamp as time spent in a stage", () => {
		topologyStore.setState(() => ({
			nodes: new Map(),
			edges: new Map(),
			version: 0,
		}));

		topologyStore.actions.ingestValues([
			{
				source: "cvd",
				timestamp: 5_000_000_000n,
			},
		]);

		const node = topologyStore.state.nodes.get("cvd");

		expect(node?.lastGapNs).toBe(0);
		expect(node?.lastAtNs).toBe(0);
		expect(node?.lastWorkNs ?? 0).toBe(0);
		expect(topologyStore.state.edges.size).toBe(0);
	});

	it("wires the measured rings and names the bottleneck", () => {
		topologyStore.setState(() => ({
			nodes: new Map(),
			edges: new Map(),
			version: 0,
		}));

		topologyStore.actions.ingestTrace({
			tick: 40,
			seq: 40,
			bytes: 18000,
			drops: 2,
			spans: [
				{
					name: "assemble",
					group: "ingress",
					stage: 0,
					waitNs: 0,
					workNs: 80_000,
					backlog: 0,
				},
				{
					name: "cvd",
					group: "signal",
					stage: 0,
					waitNs: 4_000,
					workNs: 90_000,
					backlog: 0,
				},
				{
					name: "hawkes",
					group: "signal",
					stage: 0,
					waitNs: 4_000,
					workNs: 2_400_000,
					backlog: 3,
				},
				{
					name: "training",
					group: "strategy",
					stage: 1,
					waitNs: 12_000,
					workNs: 400_000,
					backlog: 0,
				},
				{
					name: "hub",
					group: "sink",
					stage: 2,
					waitNs: 1_000,
					workNs: 30_000,
					backlog: 0,
				},
				{
					name: "book",
					group: "venue",
					stage: 0,
					waitNs: 0,
					workNs: 3_000_000,
					backlog: 0,
					side: true,
				},
			],
		});

		const { nodes, edges, account } = topologyStore.state;

		expect(edges.has("assemble>cvd")).toBe(true);
		expect(edges.has("assemble>hawkes")).toBe(true);
		expect(edges.has("cvd>hawkes")).toBe(false);
		expect(edges.has("hawkes>training")).toBe(true);
		expect(edges.has("training>hub")).toBe(true);
		expect(edges.has("book>assemble")).toBe(true);
		expect(edges.get("hawkes>training")?.lastLatencyNs).toBe(400_000);
		expect(nodes.get("hawkes")?.lastWorkNs).toBe(2_400_000);
		expect(nodes.get("hawkes")?.backlog).toBe(3);
		expect(nodes.get("book")?.side).toBe(true);
		expect(account?.bottleneck).toBe("hawkes");
		expect(account?.drops).toBe(2);
		// Book is off the path, so its 3ms does not extend the critical path.
		// The path is assemble, then the slower signal (hawkes wait+work),
		// then training, then the hub.
		expect(account?.criticalNs).toBe(80_000 + 2_404_000 + 412_000 + 31_000);
	});

	it("tracks Tee nodes and their queue backpressure", () => {
		topologyStore.setState(() => ({
			nodes: new Map(),
			edges: new Map(),
			version: 0,
		}));

		topologyStore.actions.ingestValues([
			{
				source: "ui_tee",
				at: 2_000_000_000n,
				metrics: [{ name: "backlog", raw: 42 }],
				metadata: [
					{ name: "group", value: "tees" },
					{ name: "stage", value: "4" },
				],
			},
			{
				source: "store_tee",
				at: 2_000_000_000n,
				metrics: [{ name: "backlog", raw: 1500 }],
				metadata: [
					{ name: "group", value: "tees" },
					{ name: "stage", value: "4" },
				],
			},
		]);

		const { nodes } = topologyStore.state;
		expect(nodes.has("ui_tee")).toBe(true);
		expect(nodes.has("store_tee")).toBe(true);

		const uiNode = nodes.get("ui_tee");
		expect(uiNode?.group).toBe("tees");
		expect(uiNode?.stage).toBe(4);
		expect(uiNode?.backlog).toBe(42);
		expect(uiNode?.maxBacklog).toBe(42);
		expect(uiNode?.lastValue).toBeDefined();
		expect(uiNode?.lastValue?.metrics?.[0]?.raw).toBe(42);

		const storeNode = nodes.get("store_tee");
		expect(storeNode?.group).toBe("tees");
		expect(storeNode?.stage).toBe(4);
		expect(storeNode?.backlog).toBe(1500);
		expect(storeNode?.maxBacklog).toBe(1500);
		expect(storeNode?.lastValue).toBeDefined();
		expect(storeNode?.lastValue?.metrics?.[0]?.raw).toBe(1500);
	});
});
