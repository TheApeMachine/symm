import { renderToReadableStream } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { DiagnosticsFrame } from "#/collections/types";
import { DiagnosticsDataflow } from "#/routes/diagnostics";

const FRAME: DiagnosticsFrame = {
	status: "flowing",
	at_ns: 10_000_000_000,
	started_ns: 1_000_000_000,
	stages: [
		{
			name: "correlation",
			count: 2,
			total_ns: 4_000_000,
			last_ns: 3_000_000,
			max_ns: 3_000_000,
			last_at_ns: 9_000_000_000,
		},
		{
			name: "graph",
			count: 1,
			total_ns: 5_000_000,
			last_ns: 5_000_000,
			max_ns: 5_000_000,
			last_at_ns: 9_500_000_000,
		},
	],
	hops: [],
	queues: [
		{
			name: "measurements",
			kind: "rail",
			writers: ["correlation"],
			readers: ["graph"],
			depth: 12,
			cap: 0,
			high_water: 19,
			symbols: 3,
		},
	],
	errors: [],
	pass: { state: "running", in_flight_ns: 1_000_000 },
};

const render = async (frame: DiagnosticsFrame = FRAME) => {
	const stream = await renderToReadableStream(
		<DiagnosticsDataflow frame={frame} connection="connected" />,
	);

	return new Response(stream).text();
};

describe("DiagnosticsDataflow", () => {
	it("renders the server-reported pipeline as a node graph with stages, queues, and direction", async () => {
		const markup = await render();

		expect(markup).toContain("Inspect Correlation");
		expect(markup).toContain("Measurements");
		expect(markup).toContain("Inspect Graph");
		expect(markup).toContain("Inspect queue measurements");
		// Queues render as compact tanks with a rising fill, not as oval pipes.
		expect(markup).toContain("rounded-md");
		expect(markup).not.toContain("rounded-[50%]");
		// The graph draws writer→queue and queue→reader edges on the svg; the
		// router emits explicit orthogonal line segments and edge identity.
		expect(markup).toMatch(/<path d="M [^"]*\bL\b [^"]*"/);
		expect(markup).toContain('data-from="correlation"');
		expect(markup).toContain('data-to="measurements"');
		// Edges use the muted latency-health palette, never a grey line.
		expect(markup).toContain('data-health="healthy"');
		expect(markup).toContain('stroke="hsl(140 32% 62%)"');
		// Numeric columns reserve their widest expected footprint so changing
		// counts and durations cannot move neighboring labels or controls.
		expect(markup).toContain("grid-cols-[2.5ch_7ch_6ch]");
		expect(markup).toContain("grid-cols-[minmax(0,1fr)_7ch_6ch]");
		expect(markup).toContain("tabular-nums");
		// Flow state and latency health are part of the legend.
		expect(markup).toContain("flowing");
		expect(markup).toContain("idle (solid)");
		expect(markup).toContain("healthy");
	});

	it("reports live pressure, growth, and processing averages without claiming payload quality", async () => {
		const markup = await render();

		expect(markup).toContain("pending");
		expect(markup).toContain(">12<");
		expect(markup).toContain("Correlation");
		expect(markup).toContain("2.00ms");
		expect(markup).not.toContain("Payload quality");
		expect(markup).not.toContain("Pipeline wiring map");
	});

	it("shows an in-flight stage while its first operation is still running", async () => {
		const markup = await render({
			...FRAME,
			stages: [
				{
					name: "category",
					count: 0,
					total_ns: 0,
					last_ns: 0,
					active: 1,
					started_ns: 7_000_000_000,
				},
			],
		});

		expect(markup).toContain("Inspect Category");
		expect(markup).toContain("running");
		expect(markup).toContain("3.00s");
		expect(markup).toContain("1 active");
	});

	it("keeps the empty frame inside the fixed dataflow surface", async () => {
		const markup = await render({
			...FRAME,
			stages: [],
			queues: [],
		});

		expect(markup).toContain("Waiting for queue topology");
		expect(markup).toContain("min-h-0");
		expect(markup).toContain("overflow-hidden");
	});
});
