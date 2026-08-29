import { renderToReadableStream } from "react-dom/server";
import { beforeEach, describe, expect, it } from "vitest";
import { topologyStore } from "#/collections/topology";
import { DiagnosticsGraph, type DiagnosticsSelection } from "#/components/dashboard/diagnostics-graph";
import type { EnvelopeBoundaryStamp } from "#/providers/telemetry/telemetry/envelope-boundary-stamp";

/*
FakeStamp mimics only the EnvelopeBoundaryStamp accessors topologyStore.ingest
reads (label/atNs/seqCount/avgGapNs/lastGapNs/backlog) — the generated
FlatBuffers view class needs a real backing buffer to construct, so tests
build the same shape by hand rather than encoding one.
*/
class FakeStamp {
	constructor(
		private readonly _label: string,
		private readonly _atNs: bigint,
		private readonly _seqCount = 1n,
		private readonly _avgGapNs = 0n,
		private readonly _lastGapNs = 0n,
		private readonly _backlog = 0n,
	) {}

	label() {
		return this._label;
	}
	atNs() {
		return this._atNs;
	}
	seqCount() {
		return this._seqCount;
	}
	avgGapNs() {
		return this._avgGapNs;
	}
	lastGapNs() {
		return this._lastGapNs;
	}
	backlog() {
		return this._backlog;
	}
}

const ingest = (
	trace: [string, number, number?][],
) => {
	const stamps = trace.map(
		([label, atNs, backlog]) =>
			new FakeStamp(label, BigInt(atNs), 1n, 0n, 0n, BigInt(backlog ?? 0)),
	);

	// FakeStamp satisfies the accessor shape ingest() actually calls
	// (label/atNs/seqCount/avgGapNs/lastGapNs/backlog) but isn't the real
	// generated EnvelopeBoundaryStamp class, so it can only be asserted via
	// unknown rather than structurally matching the type.
	topologyStore.actions.ingest(stamps as unknown as EnvelopeBoundaryStamp[]);
};

const render = async (
	selection: DiagnosticsSelection | null = null,
	atNs?: number,
) => {
	const { nodes, edges } = topologyStore.state;
	const resolvedAtNs =
		atNs ?? Math.max(0, ...Array.from(nodes.values()).map((n) => n.lastAtNs));
	const stream = await renderToReadableStream(
		<DiagnosticsGraph nodes={nodes} edges={edges} atNs={resolvedAtNs} selection={selection} onSelect={() => {}} />,
	);

	return new Response(stream).text();
};

describe("DiagnosticsGraph", () => {
	beforeEach(() => {
		topologyStore.setState(() => ({ nodes: new Map(), edges: new Map(), version: 0 }));
	});

	it("renders a stage discovered purely from boundary stamps", async () => {
		ingest([
			["ticker.ingress", 10_000_000_000],
			["ticker.signals", 10_000_050_000],
		]);

		const markup = await render();

		expect(markup).toContain("Inspect ticker.ingress");
		expect(markup).toContain("Inspect ticker.signals");
		expect(markup).toMatch(/<path d="M [^"]*\bL\b [^"]*"/);
		expect(markup).toContain('data-from="ticker.ingress"');
		expect(markup).toContain('data-to="ticker.signals"');
	});

	it("shows the average hop latency between two consecutive stamps", async () => {
		ingest([
			["a", 10_000_000_000],
			["b", 10_000_042_000],
		]);

		const markup = await render();

		expect(markup).toContain("42.0µs");
	});

	it("keeps stages that have gone stale rendered, just dimmed to the stale tone", async () => {
		ingest([["only", 0]]);

		// Far beyond TOPOLOGY_LIVE_WINDOW_NS (2s) past the stamp's own atNs, so
		// the stage reads as stale rather than live.
		const markup = await render(null, 10_000_000_000);

		expect(markup).toContain("Inspect only");
		expect(markup).toContain("stale");
	});

	it("renders the empty-topology placeholder before any envelope has been stamped", async () => {
		const markup = await render();

		expect(markup).toContain("Waiting for the pipeline to stamp its first boundary");
	});

	it("shows real ring backlog stamped from the Workload's own sequence numbers", async () => {
		ingest([["slow.stage", 10_000_000_000, 5]]);

		const markup = await render();

		expect(markup).toContain("bklg");
		expect(markup).toContain(">5<");
	});
});
