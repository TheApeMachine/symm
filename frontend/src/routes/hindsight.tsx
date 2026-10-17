import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useEffect, useMemo, useRef, useState } from "react";
import {
	beginHindsight,
	type HindsightExcursion,
	type HindsightStream,
	hindsightStore,
} from "#/collections/hindsight";
import {
	annotationBody,
	DETECTION_CLASSES,
	type DetectionClass,
	draftOf,
	emptyAnnotation,
	MARKERS,
	type Marker,
	marksOf,
	nextMarker,
	placeMarker,
	snapTick,
} from "#/components/hindsight/annotate";
import {
	fetchHindsightRuns,
	summariesOf,
	timelineOf,
} from "#/components/hindsight/hindsight-api";
import type {
	HindsightEpisode,
	HindsightRun,
	MarketCoordinate,
	TimelineAxis,
} from "#/components/hindsight/hindsight-types";
import { EpisodeTargets, SymbolTargets } from "#/components/hindsight/targets";
import { Overview, Timeline } from "#/components/hindsight/timeline";
import {
	Badge,
	Button,
	Flex,
	Section,
	Toolbar,
	Typography,
} from "#/components/ui";
import { hubBaseUrl } from "#/lib/hub";
import { sendCommand } from "#/providers/websocket";

/*
Hindsight reads one stored run back: the ticks collect wrote, the
excursions detect found, the policies train published for the run and the
paper records traded under them. The run list is REST (/hindsight/runs);
the run index and each symbol stream over the hub socket.
*/

const COORDINATES: MarketCoordinate[] = ["midpoint", "trade", "bid", "ask"];
const AXES: TimelineAxis[] = ["time", "capture"];
const DETAIL_BUCKETS = 320;
const OVERVIEW_BUCKETS = 200;

/*
chooseRun opens the collector's tape. That is the run with the most stored
ticks. A hub that has not reported counts yet still lists the collector first
by start time: the dashboard's own runs are the later, shorter ones.
*/
const chooseRun = (loaded: HindsightRun[]): string | null => {
	if (loaded.length === 0) return null;

	const longest = loaded.reduce((best, run) =>
		(run.ticks ?? 0) > (best.ticks ?? 0) ? run : best,
	);

	if ((longest.ticks ?? 0) > 0) return longest.id;

	return loaded.reduce((best, run) =>
		Date.parse(run.startedAt) < Date.parse(best.startedAt) ? run : best,
	).id;
};

const runLabel = (run: HindsightRun): string => {
	const at = new Date(run.startedAt);
	const when = Number.isNaN(at.getTime())
		? run.startedAt
		: at.toLocaleString([], {
				hour12: false,
				month: "2-digit",
				day: "2-digit",
				hour: "2-digit",
				minute: "2-digit",
				second: "2-digit",
			});
	const ticks =
		run.ticks !== undefined && run.ticks > 0
			? ` · ${run.ticks.toLocaleString()} ticks`
			: "";

	return `${when}${ticks}`;
};

const send = (run: string, symbol: string) => {
	try {
		sendCommand(beginHindsight(run, symbol));
	} catch (cause) {
		console.error(cause);
		hindsightStore.setState((state) => {
			const slot = symbol === "" ? "index" : "stream";
			const stream = state[slot];
			return stream === null
				? state
				: {
						...state,
						[slot]: { ...stream, status: "error", error: String(cause) },
					};
		});
	}
};

const StreamStatus = ({
	label,
	stream,
}: {
	label: string;
	stream: HindsightStream | null;
}) => {
	if (stream === null) return null;

	const variant =
		stream.status === "error"
			? "error"
			: stream.status === "done"
				? "success"
				: "warning";
	const text =
		stream.status === "streaming"
			? `${label}: streaming ${stream.ticks.length || stream.symbols.length}`
			: stream.status === "done"
				? `${label}: ${stream.counts?.ticks ?? 0} ticks`
				: `${label}: failed`;

	return <Badge size="xs" variant={variant} label={text} />;
};

const ExcursionDetail = ({ excursion }: { excursion: HindsightExcursion }) => {
	const f = excursion.fields;
	const rows: Array<[string, string]> = [
		["key", excursion.key],
		[
			"A / B / C / D tick",
			`${f.a_tick} / ${f.b_tick} / ${f.c_tick} / ${f.d_tick}`,
		],
		[
			"B → C price",
			`${f.b_price} → ${f.c_price} (${(((f.c_price - f.b_price) / f.b_price) * 100).toFixed(3)}%)`,
		],
		["entry tick · ask", `${f.entry_tick} · ${f.entry_ask}`],
		["exit tick · bid", `${f.exit_tick} · ${f.exit_bid}`],
		[
			"entry → exit",
			`${(((f.exit_bid - f.entry_ask) / f.entry_ask) * 100).toFixed(3)}% before fees`,
		],
	];

	return (
		<Section fit="content" className="shrink-0">
			<Section.Header
				size="s"
				title={`Excursion · ${excursion.kind}`}
				meta="as detect stored it"
			/>
			<div className="grid grid-cols-[120px_1fr] gap-x-2 gap-y-0.5 p-2 font-mono text-[10px]">
				{rows.map(([name, value]) => (
					<div key={name} className="contents">
						<span className="text-(--f4)">{name}</span>
						<span className="break-all text-(--f1)">{value}</span>
					</div>
				))}
			</div>
		</Section>
	);
};

const Policies = ({ stream }: { stream: HindsightStream }) => (
	<Section fit="content" className="shrink-0">
		<Section.Header
			size="s"
			title="Policies"
			meta={
				stream.policies.length === 0
					? "train published none for this run"
					: `${stream.policies.length} paths`
			}
		/>
		<div className="max-h-48 overflow-y-auto font-mono text-[10px]">
			{stream.policies
				.slice()
				.sort((left, right) => right.fields.gain - left.fields.gain)
				.map((policy) => (
					<div
						key={`${policy.action}:${policy.path}`}
						className="grid grid-cols-[1fr_44px_60px_44px_44px] gap-1.5 px-2 py-0.5"
					>
						<span className="truncate text-(--f1)">{policy.path}</span>
						<span className="text-(--f3)">{policy.action}</span>
						<span
							className={
								policy.fields.gain >= 0 ? "text-emerald-400" : "text-rose-400"
							}
						>
							{(policy.fields.gain * 100).toFixed(3)}%
						</span>
						<span className="text-(--f4)">n {policy.fields.visits}</span>
						<span className="text-(--f4)">
							c {policy.fields.confidence.toFixed(2)}
						</span>
					</div>
				))}
		</div>
	</Section>
);

const Papers = ({ stream }: { stream: HindsightStream }) => (
	<Section fit="content" className="shrink-0">
		<Section.Header
			size="s"
			title="Paper trades"
			meta={
				stream.papers.length === 0
					? "none uploaded under this run's policies"
					: `${stream.papers.length} decision records`
			}
		/>
		<div className="max-h-48 overflow-y-auto font-mono text-[10px]">
			{stream.papers.map((record) => (
				<div
					key={record.seq}
					className="grid grid-cols-[38px_1fr_60px_60px] gap-1.5 px-2 py-0.5"
				>
					<span className="text-(--f3)">{record.action}</span>
					<span className="truncate text-(--f1)">
						{record.path} · d{record.fields.depth}
					</span>
					<span className="text-(--f4)">
						{(record.fields.slippage * 100).toFixed(3)}%
					</span>
					<span
						className={
							record.fields.pnl >= 0 ? "text-emerald-400" : "text-rose-400"
						}
					>
						{(record.fields.net_return * 100).toFixed(3)}%
					</span>
				</div>
			))}
		</div>
	</Section>
);

const HindsightRoute = () => {
	const [runs, setRuns] = useState<HindsightRun[]>([]);
	const [runsError, setRunsError] = useState<string | null>(null);
	const [run, setRun] = useState<string | null>(null);
	const [symbol, setSymbol] = useState<string | null>(null);
	const [coordinate, setCoordinate] = useState<MarketCoordinate>("midpoint");
	const [axis, setAxis] = useState<TimelineAxis>("time");
	const [viewport, setViewport] = useState<{ from: number; to: number } | null>(
		null,
	);
	const [playhead, setPlayhead] = useState<number | null>(null);
	const [episode, setEpisode] = useState<string | null>(null);
	const [marker, setMarker] = useState<Marker>("A");
	const [annotation, setAnnotation] = useState(emptyAnnotation);
	const [detectionClass, setDetectionClass] = useState<DetectionClass>("up");
	const [saveError, setSaveError] = useState<string | null>(null);
	const [busy, setBusy] = useState<string | null>(null);
	const chosen = useRef(false);
	const { index, stream } = useSelector(hindsightStore, (state) => state);

	useEffect(() => {
		let cancelled = false;

		fetchHindsightRuns()
			.then((loaded) => {
				if (cancelled) return;
				setRuns(loaded);
				setRun((current) => current ?? chooseRun(loaded));
			})
			.catch((cause: unknown) => {
				if (!cancelled)
					setRunsError(cause instanceof Error ? cause.message : String(cause));
			});

		return () => {
			cancelled = true;
		};
	}, []);

	// The run index names every symbol. Opening one symbol streams that tape beside it.
	useEffect(() => {
		if (run === null) return;
		chosen.current = false;
		setSymbol(null);
		setViewport(null);
		setPlayhead(null);
		setEpisode(null);
		send(run, "");
	}, [run]);

	const summaries = useMemo(() => summariesOf(index), [index]);
	const indexRun = index?.run ?? null;

	// Open the first instrument as soon as it is named. When the index
	// finishes, move to the one that travelled furthest unless a click chose.
	useEffect(() => {
		if (chosen.current || indexRun !== run || summaries.length === 0) return;

		if (index?.status === "done") {
			const top = summaries.reduce((best, row) =>
				row.topExcursion > best.topExcursion ? row : best,
			);
			setSymbol(top.symbol);
			return;
		}

		if (symbol === null) setSymbol(summaries[0].symbol);
	}, [index, indexRun, run, symbol, summaries]);

	useEffect(() => {
		if (run === null || symbol === null) return;
		if (indexRun !== null && indexRun !== run) return;
		setViewport(null);
		setPlayhead(null);
		setEpisode(null);
		send(run, symbol);
	}, [run, symbol, indexRun]);

	const current =
		stream !== null && stream.run === run && stream.symbol === symbol
			? stream
			: null;
	const runMeta = runs.find((entry) => entry.id === run) ?? null;
	const tape = useMemo(
		() =>
			runMeta !== null && (runMeta.ticks ?? 0) > 1
				? { from: 1, to: runMeta.ticks ?? 1 }
				: null,
		[runMeta],
	);
	const overview = useMemo(
		() =>
			current
				? timelineOf(current, index, coordinate, axis, OVERVIEW_BUCKETS, tape)
				: null,
		[current, index, coordinate, axis, tape],
	);
	const detail = useMemo(
		() =>
			current
				? timelineOf(
						current,
						index,
						coordinate,
						axis,
						DETAIL_BUCKETS,
						viewport ?? tape,
					)
				: null,
		[current, index, coordinate, axis, viewport, tape],
	);

	const selected = useMemo(
		() =>
			current?.excursions.find((excursion) => excursion.key === episode) ??
			null,
		[current, episode],
	);

	const failure = runsError ?? index?.error ?? current?.error ?? null;
	const draft =
		run !== null && symbol !== null
			? annotationBody(run, symbol, detectionClass, annotation)
			: null;
	const pickedDraft =
		current === null ? null : draftOf(current.excursions, detectionClass);
	const draftMarks =
		pickedDraft === null
			? null
			: [
					Math.round(pickedDraft.fields.a_tick),
					Math.round(pickedDraft.fields.b_tick),
					Math.round(pickedDraft.fields.c_tick),
					Math.round(pickedDraft.fields.d_tick),
				].join(":");

	useEffect(() => {
		if (draftMarks === null) return;

		const [a, b, c, d] = draftMarks.split(":").map(Number);
		if (
			a === undefined ||
			b === undefined ||
			c === undefined ||
			d === undefined ||
			Number.isNaN(a) ||
			Number.isNaN(b) ||
			Number.isNaN(c) ||
			Number.isNaN(d)
		)
			return;

		setAnnotation({ A: a, B: b, C: c, D: d });
		setMarker("A");
	}, [draftMarks]);

	const postRun = async (path: string, label: string, body: unknown) => {
		if (run === null || busy !== null) return;
		setBusy(label);
		setSaveError(null);

		try {
			const response = await fetch(`${hubBaseUrl()}${path}`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify(body),
			});

			if (!response.ok) {
				throw new Error(await response.text());
			}

			const payload = (await response.json()) as {
				marker?: { a: string; b: string; c: string; d: string };
			};

			if (payload.marker !== undefined) {
				setAnnotation({
					A: Number(payload.marker.a),
					B: Number(payload.marker.b),
					C: Number(payload.marker.c),
					D: Number(payload.marker.d),
				});
			}

			if (path !== "/hindsight/commit" && symbol !== null) send(run, symbol);
		} catch (cause) {
			setSaveError(cause instanceof Error ? cause.message : String(cause));
		} finally {
			setBusy(null);
		}
	};

	return (
		<Flex.Column
			fullWidth
			className="h-full min-h-0 bg-(--bg) font-mono text-(--f2)"
		>
			<Toolbar
				size="s"
				className="gap-3 border-(--line) border-b bg-(--surface) px-3"
			>
				<Typography.Label
					size="s"
					tone="f1"
					className="font-bold tracking-wider"
				>
					Hindsight
				</Typography.Label>
				<select
					aria-label="run"
					className="bg-(--sunken) text-[10px]"
					value={run ?? ""}
					onChange={(event) => setRun(event.currentTarget.value)}
				>
					{runs.map((entry) => (
						<option key={entry.id} value={entry.id}>
							{runLabel(entry)}
							{entry.trained ? " · trained" : ""}
						</option>
					))}
				</select>
				{runMeta !== null ? (
					<Badge
						size="xs"
						variant={runMeta.trained ? "success" : "disabled"}
						label={policyLabel(runMeta)}
					/>
				) : null}
				<StreamStatus label="index" stream={index} />
				<StreamStatus label={symbol ?? "symbol"} stream={current} />
				<Button
					size="xs"
					variant="outline"
					disabled={run === null || busy !== null}
					onClick={() =>
						void postRun("/hindsight/scan", "scanning the tape", { run })
					}
				>
					scan
				</Button>
				<Toolbar.Spacer />
				{COORDINATES.map((entry) => (
					<Button
						key={entry}
						size="xs"
						variant={entry === coordinate ? "solid" : "outline"}
						onClick={() => setCoordinate(entry)}
					>
						{entry}
					</Button>
				))}
				{AXES.map((entry) => (
					<Button
						key={entry}
						size="xs"
						variant={entry === axis ? "solid" : "outline"}
						onClick={() => setAxis(entry)}
					>
						{entry}
					</Button>
				))}
			</Toolbar>

			<Toolbar
				size="s"
				className="gap-2 border-(--line) border-b bg-(--sunken) px-3"
			>
				{DETECTION_CLASSES.map((entry) => (
					<Button
						key={entry}
						size="xs"
						variant={entry === detectionClass ? "solid" : "outline"}
						onClick={() => setDetectionClass(entry)}
					>
						{entry}
					</Button>
				))}
				{MARKERS.map((entry) => (
					<Button
						key={entry}
						size="xs"
						variant={entry === marker ? "solid" : "outline"}
						onClick={() => setMarker(entry)}
					>
						{entry}
						{annotation[entry] === null ? "" : ` ${annotation[entry]}`}
					</Button>
				))}
				<Button
					size="xs"
					variant="solid"
					disabled={draft === null || busy !== null}
					onClick={() =>
						void postRun("/hindsight/commit", "recalculating detections", draft)
					}
				>
					commit
				</Button>
				<Button
					size="xs"
					variant="outline"
					disabled={run === null || busy !== null}
					onClick={() =>
						void postRun("/hindsight/commit-all", "committing the tape", {
							run,
						})
					}
				>
					commit all
				</Button>
				<Typography.Label size="s" tone="f4">
					drag a marker, or click the tape to move {marker}
				</Typography.Label>
				{saveError !== null ? (
					<span className="text-[10px] text-rose-300">{saveError}</span>
				) : null}
			</Toolbar>

			{failure !== null ? (
				<div
					role="alert"
					className="border-(--line) border-b bg-rose-950/40 px-3 py-1.5 text-[10px] text-rose-300"
				>
					{failure}
				</div>
			) : null}

			{runs.length === 0 && runsError === null ? (
				<Flex.Center className="flex-1 text-(--f4) text-xs">
					no stored runs: run collect first
				</Flex.Center>
			) : (
				<div className="grid min-h-0 flex-1 grid-cols-[220px_minmax(0,1fr)_320px]">
					<Flex.Column className="min-h-0 border-(--line) border-r">
						<SymbolTargets
							summaries={summaries}
							selected={symbol}
							onSelect={(next) => {
								chosen.current = true;
								setSymbol(next);
							}}
							pending={
								index?.status === "error"
									? "The run index stopped."
									: index?.status === "done"
										? "No instrument of this run has a stored up or down excursion."
										: "Reading this run's tape."
							}
						/>
					</Flex.Column>
					<Flex.Column className="relative h-full min-h-0 overflow-hidden">
						{busy !== null ? (
							<div className="absolute inset-0 z-10 flex items-center justify-center gap-2 bg-black/60">
								<div className="h-5 w-5 animate-spin rounded-full border-2 border-(--f4) border-t-(--f1)" />
								<span className="text-[10px] text-(--f1)">{busy}</span>
							</div>
						) : null}
						<Overview
							timeline={overview}
							window={viewport}
							onZoom={(from, to) => setViewport({ from, to })}
							onResetZoom={() => setViewport(null)}
						/>
						<Timeline
							timeline={detail}
							emptyLabel={
								symbol !== null && current?.status === "streaming"
									? `Reading ${symbol}.`
									: symbol === null && index?.status !== "error"
										? "Reading this run's tape."
										: undefined
							}
							gaps={[]}
							lifecycle={[]}
							positions={[]}
							playhead={playhead}
							marks={marksOf(annotation)}
							selectedEpisode={episode}
							selectedPosition={null}
							onMark={(label, sequence) => {
								if (current === null) return;
								const snapped = snapTick(
									current.ticks.map((tick) => tick.tick),
									sequence,
								);
								if (snapped === null) return;
								if (!MARKERS.includes(label as Marker)) return;
								setAnnotation((placed) =>
									placeMarker(placed, label as Marker, snapped),
								);
							}}
							onPlayhead={(sequence) => {
								if (current === null) {
									setPlayhead(sequence);
									return;
								}

								const snapped = snapTick(
									current.ticks.map((tick) => tick.tick),
									sequence,
								);
								if (snapped === null) return;
								setAnnotation((placed) => placeMarker(placed, marker, snapped));
								setMarker(nextMarker(marker));
								setSaveError(null);
							}}
							onEpisode={(picked: HindsightEpisode) => {
								setEpisode(picked.id);
								setViewport({
									from: picked.fromSequence,
									to: picked.toSequence,
								});
							}}
							onPosition={() => {}}
							onZoom={(from, to) => setViewport({ from, to })}
							onResetZoom={() => setViewport(null)}
						/>
						{selected !== null ? (
							<ExcursionDetail excursion={selected} />
						) : null}
					</Flex.Column>
					<Flex.Column className="min-h-0 overflow-y-auto border-(--line) border-l">
						<EpisodeTargets
							timeline={detail}
							selected={episode}
							onSelect={(picked) => {
								setEpisode(picked.id);
								setViewport({
									from: picked.fromSequence,
									to: picked.toSequence,
								});
							}}
							onReference={(reference) => setPlayhead(reference.sequence)}
						/>
						{current !== null ? (
							<>
								<Policies stream={current} />
								<Papers stream={current} />
							</>
						) : null}
					</Flex.Column>
				</div>
			)}
		</Flex.Column>
	);
};

/*
policyLabel names the policy epoch the stream replays (the newest trained on
the run) and the gate it was trained under, and how many others exist.
*/
const policyLabel = (run: HindsightRun) => {
	const [newest, ...older] = run.policies ?? [];

	if (!newest) return "no policies";

	const gate = newest.gate
		? `gated z≥${newest.surpriseZ}, warm-up ${newest.warmup}`
		: "ungated";

	return `policies ${newest.epoch} · ${gate}${older.length > 0 ? ` · +${older.length} older` : ""}`;
};

export const Route = createFileRoute("/hindsight")({
	component: HindsightRoute,
});
