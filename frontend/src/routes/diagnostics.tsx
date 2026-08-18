import { createFileRoute } from "@tanstack/react-router";
import { useLayoutEffect, useRef, useState } from "react";
import type {
	ClockSnapshot,
	DiagnosticsFrame,
	HopSnapshot,
	LaneSnapshot,
} from "#/collections/types";
import { DiagnosticsSchema } from "#/components/dashboard/diagnostics-schema";
import { Flex } from "#/components/ui/flex";
import type { JSONSerializable } from "#/components/ui/paint";
import { Panel } from "#/components/ui/panel";
import { getLastFrame, registerPainter } from "#/providers/ws-stores";

const DEFAULT_FRAME: DiagnosticsFrame = {
	status: "flowing",
	summary:
		"In-flight work is moving. A few events in a lane is the healthy plane, not a stall.",
	ingress_sequence: 0,
	committed_sequence: 0,
	next_sequence: 1,
	lag: 0,
	pending: 0,
	dropped: 0,
	commit_dropped: 0,
	tickers: 0,
	books: 0,
	trades: 0,
	level3: 0,
	coalesced_books: 0,
	stall_ns: 0,
	ui_depth: 0,
	ui_cap: 0,
	ui_sent: 0,
	ui_dropped: 0,
	lanes: [],
	lossy: false,
	at_ns: 0,
	started_ns: 0,
	stages: [],
	hops: [],
};

const STATUS_TONE = {
	flowing: {
		bg: "bg-(--up)",
		text: "text-(--up)",
		ring: "shadow-[0_0_15px_rgba(34,197,94,0.35)]",
		label: "Flowing",
	},
	queued: {
		bg: "bg-(--warn)",
		text: "text-(--warn)",
		ring: "shadow-[0_0_15px_rgba(234,179,8,0.35)]",
		label: "Queued",
	},
	lossy: {
		bg: "bg-(--warn)",
		text: "text-(--warn)",
		ring: "shadow-[0_0_15px_rgba(234,179,8,0.35)]",
		label: "Lossy",
	},
	stalled: {
		bg: "bg-(--down)",
		text: "text-(--down)",
		ring: "shadow-[0_0_15px_rgba(239,68,68,0.35)]",
		label: "Stalled",
	},
} as const;

type RateSample = {
	atNs: number;
	committed: number;
	tickers: number;
	books: number;
	trades: number;
	level3: number;
};

type StatusNote = {
	at: string;
	status: string;
	summary: string;
};

const perSecond = (
	next: number,
	previous: number,
	elapsedNs: number,
): number => {
	if (elapsedNs <= 0 || next < previous) {
		return 0;
	}

	return ((next - previous) * 1_000_000_000) / elapsedNs;
};

const formatRate = (rate: number): string => {
	if (rate <= 0) {
		return "0/s";
	}

	if (rate < 10) {
		return `${rate.toFixed(1)}/s`;
	}

	return `${rate.toFixed(0)}/s`;
};

const formatDuration = (nanos: number): string => {
	if (nanos <= 0) {
		return "0s";
	}

	const millis = nanos / 1_000_000;

	if (millis < 1000) {
		return `${millis.toFixed(0)}ms`;
	}

	return `${(millis / 1000).toFixed(2)}s`;
};

const fillRatio = (lane: LaneSnapshot): number => {
	if (lane.capacity <= 0) {
		return 0;
	}

	return Math.min(1, lane.depth / lane.capacity);
};

const DiagnosticsBridge = ({
	onFrame,
}: {
	onFrame: (frame: DiagnosticsFrame) => void;
}) => {
	useLayoutEffect(() => {
		const paint = (updates: JSONSerializable) => {
			if (updates && typeof updates === "object" && !Array.isArray(updates)) {
				onFrame(updates as DiagnosticsFrame);
			}
		};

		const unregister = registerPainter("diagnostics", paint);
		const seed = getLastFrame("diagnostics");

		if (seed && typeof seed === "object" && !Array.isArray(seed)) {
			onFrame(seed as DiagnosticsFrame);
		}

		return unregister;
	}, [onFrame]);

	return null;
};

const LaneRow = ({ lane }: { lane: LaneSnapshot }) => {
	const ratio = fillRatio(lane);
	const saturated =
		lane.blocking && lane.capacity > 0 && lane.depth >= lane.capacity;
	const barTone = saturated
		? "bg-(--down)"
		: ratio > 0.5
			? "bg-(--warn)"
			: "bg-(--acc)";

	return (
		<div className="grid grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_72px_72px_88px] items-center gap-3 border-(--line) border-b py-1.5 last:border-b-0">
			<Flex.Column className="min-w-0">
				<span className="truncate font-mono text-[11px] text-(--f1)">
					{lane.name}
				</span>
				<span className="font-mono text-[9px] uppercase tracking-wider text-(--f4)">
					{lane.kind}
					{lane.blocking ? " · blocks producer" : " · droppable"}
				</span>
			</Flex.Column>
			<div className="h-1.5 overflow-hidden rounded-full bg-(--sunken)">
				<div
					className={`h-full ${barTone}`}
					style={{ width: `${(ratio * 100).toFixed(1)}%` }}
				/>
			</div>
			<span className="text-right font-mono text-[11px] text-(--f1)">
				{lane.depth}/{lane.capacity}
			</span>
			<span className="text-right font-mono text-[11px] text-(--f3)">
				{lane.high_water}
			</span>
			<span className="text-right font-mono text-[11px] text-(--f3)">
				{lane.saturations} · {formatDuration(lane.saturation_ns)}
			</span>
		</div>
	);
};

const RateCell = ({ label, rate }: { label: string; rate: string }) => (
	<Flex.Column className="min-w-20">
		<span className="font-mono text-[9px] uppercase tracking-wider text-(--f4)">
			{label}
		</span>
		<span className="font-mono text-[13px] tabular-nums text-(--f1)">
			{rate}
		</span>
	</Flex.Column>
);

const DiagnosticsSurface = () => {
	const [frame, setFrame] = useState<DiagnosticsFrame>(DEFAULT_FRAME);
	const [rates, setRates] = useState({
		committed: 0,
		tickers: 0,
		books: 0,
		trades: 0,
		level3: 0,
	});
	const [notes, setNotes] = useState<StatusNote[]>([]);
	const previous = useRef<RateSample | null>(null);
	const lastNote = useRef("");

	const onFrame = (next: DiagnosticsFrame) => {
		const sample: RateSample = {
			atNs: next.at_ns ?? 0,
			committed: next.committed_sequence ?? 0,
			tickers: next.tickers ?? 0,
			books: next.books ?? 0,
			trades: next.trades ?? 0,
			level3: next.level3 ?? 0,
		};
		const prior = previous.current;

		if (prior && sample.atNs > prior.atNs) {
			const elapsed = sample.atNs - prior.atNs;
			setRates({
				committed: perSecond(sample.committed, prior.committed, elapsed),
				tickers: perSecond(sample.tickers, prior.tickers, elapsed),
				books: perSecond(sample.books, prior.books, elapsed),
				trades: perSecond(sample.trades, prior.trades, elapsed),
				level3: perSecond(sample.level3, prior.level3, elapsed),
			});
		}

		previous.current = sample;

		if (
			next.status === "stalled" &&
			next.summary &&
			next.summary !== lastNote.current
		) {
			lastNote.current = next.summary;
			const stamp = new Date().toLocaleTimeString();
			setNotes((current) =>
				[
					{
						at: stamp,
						status: next.status ?? "stalled",
						summary: next.summary ?? "",
					},
					...current,
				].slice(0, 8),
			);
		}

		setFrame(next);
	};

	const status = frame.status === "stalled" ? "stalled" : "flowing";
	const tone = STATUS_TONE[status];
	const lanes = frame.lanes ?? [];
	const stages = frame.stages ?? [];
	const hops = frame.hops ?? [];
	const lifetimeNs =
		(frame.at_ns ?? 0) > 0 && (frame.started_ns ?? 0) > 0
			? (frame.at_ns ?? 0) - (frame.started_ns ?? 0)
			: 0;
	const lifetimeRate = perSecond(frame.committed_sequence ?? 0, 0, lifetimeNs);

	return (
		<div className="flex h-full min-w-275 flex-col overflow-auto bg-(--bg) p-5 gap-5">
			<DiagnosticsBridge onFrame={onFrame} />

			<Panel className="flex flex-col gap-4 rounded-md border border-(--line2) bg-(--surface) p-5">
				<Flex.Row justify="between" align="center" className="w-full">
					<Flex.Column gap={1}>
						<span className="font-mono text-[10px] uppercase tracking-widest text-(--f4)">
							Analytical data plane
						</span>
						<h1 className="text-xl font-bold tracking-tight text-(--f1)">
							System diagnostics
						</h1>
					</Flex.Column>
					<Flex.Row align="center" gap={3} className="shrink-0">
						<div
							className={`relative h-4 w-4 rounded-full ${tone.bg} ${tone.ring}`}
						/>
						<span
							className={`font-mono text-[13px] font-semibold uppercase tracking-wider ${tone.text}`}
						>
							{tone.label}
						</span>
					</Flex.Row>
				</Flex.Row>

				<p className="font-mono text-[12px] leading-relaxed text-(--f3)">
					{status === "stalled"
						? (frame.summary ?? DEFAULT_FRAME.summary)
						: DEFAULT_FRAME.summary}
				</p>

				<Flex.Row className="flex-wrap gap-5 border-(--line) border-t pt-3">
					<RateCell label="commit" rate={formatRate(rates.committed)} />
					<RateCell label="lifetime" rate={formatRate(lifetimeRate)} />
					<RateCell label="tickers" rate={formatRate(rates.tickers)} />
					<RateCell label="books" rate={formatRate(rates.books)} />
					<RateCell label="trades" rate={formatRate(rates.trades)} />
					<RateCell label="l3" rate={formatRate(rates.level3)} />
					<RateCell label="lag" rate={`${frame.lag ?? 0}`} />
					<RateCell
						label="drops"
						rate={`${frame.dropped ?? 0}/${frame.commit_dropped ?? 0}`}
					/>
					<RateCell
						label="ui queue"
						rate={`${frame.ui_depth ?? 0}/${frame.ui_cap ?? 0}`}
					/>
					<RateCell label="ui drops" rate={`${frame.ui_dropped ?? 0}`} />
				</Flex.Row>
			</Panel>

			<Panel className="flex flex-col gap-3 rounded-md border border-(--line) bg-(--surface) p-4">
				<Flex.Row justify="between" align="center">
					<span className="font-mono text-[10px] font-semibold uppercase tracking-wider text-(--f3)">
						Stage map
					</span>
					<span className="font-mono text-[9px] uppercase tracking-wider text-(--f4)">
						node = average work · edge = wait between stages
					</span>
				</Flex.Row>
				<DiagnosticsSchema
					stages={stages as ClockSnapshot[]}
					hops={hops as HopSnapshot[]}
					atNs={frame.at_ns ?? 0}
				/>
			</Panel>

			<Panel className="flex flex-col gap-2 rounded-md border border-(--line) bg-(--surface) p-4">
				<Flex.Row justify="between" align="center">
					<span className="font-mono text-[10px] font-semibold uppercase tracking-wider text-(--f3)">
						Lanes
					</span>
					<span className="font-mono text-[9px] uppercase tracking-wider text-(--f4)">
						depth / cap · high water · saturations
					</span>
				</Flex.Row>
				{lanes.length === 0 ? (
					<p className="font-mono text-[11px] text-(--f4)">
						No lane snapshot yet. The publisher emits on the bus heartbeat.
					</p>
				) : (
					<div>
						{lanes.map((lane) => (
							<LaneRow key={lane.name} lane={lane} />
						))}
					</div>
				)}
			</Panel>

			<Panel className="flex flex-col gap-2 rounded-md border border-(--line) bg-(--surface)/50 p-4">
				<span className="font-mono text-[10px] font-semibold uppercase tracking-wider text-(--f3)">
					Stall log
				</span>
				{notes.length === 0 ? (
					<p className="font-mono text-[11px] text-(--f4)">
						No stall has held across a heartbeat. In-flight depth is shown on
						the lanes, not in this log.
					</p>
				) : (
					<div className="flex flex-col gap-1.5">
						{notes.map((note) => (
							<div
								key={`${note.at}-${note.summary}`}
								className="font-mono text-[11px] text-(--f3)"
							>
								<span className="text-(--f4)">{note.at}</span>{" "}
								<span className="uppercase text-(--down)">{note.status}</span>{" "}
								{note.summary}
							</div>
						))}
					</div>
				)}
			</Panel>
		</div>
	);
};

export const Route = createFileRoute("/diagnostics")({
	component: DiagnosticsSurface,
});
