import { createFileRoute } from "@tanstack/react-router";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
	fetchHindsightCaptures,
	fetchHindsightEnvelope,
	fetchHindsightGaps,
	fetchHindsightLifecycle,
	fetchHindsightMetricMap,
	fetchHindsightResident,
	fetchHindsightRuns,
	fetchHindsightState,
	fetchHindsightTimeline,
} from "#/components/hindsight/hindsight-api";
import type {
	HindsightCapture,
	HindsightEnvelope,
	HindsightEpisode,
	HindsightGap,
	HindsightLifecycleEvent,
	HindsightMetricMap,
	HindsightRef,
	HindsightResident,
	HindsightRun,
	HindsightTimeline,
	MarketCoordinate,
	TimelineAxis,
} from "#/components/hindsight/hindsight-types";
import {
	type CompareMode,
	ComparePanel,
	type Mark,
	MarkBar,
} from "#/components/hindsight/compare";
import {
	compareHindsightRef,
	orderHindsightRefs,
} from "#/components/hindsight/hindsight-types";
import {
	CaptureCard,
	decodeEnvelopeState,
	FrameStrip,
	ProvenancePanel,
	StatePanel,
} from "#/components/hindsight/inspector";
import {
	EpisodeTargets,
	SymbolTargets,
} from "#/components/hindsight/targets";
import {
	buildPositions,
	type Position,
} from "#/components/hindsight/positions";
import { Overview, Timeline } from "#/components/hindsight/timeline";
import { formatClock, formatCount } from "#/components/hindsight/timeline-scale";
import type { EnvelopeState } from "#/providers/telemetry/telemetry/envelope-state";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";

/*
Hindsight — a microscope over a captured running system.

The market tells us which slides are interesting; the capture tape tells us what
reality reached SYMM; the witness tape tells us what SYMM produced from it. This
surface puts those three in one horizontal frame: a declared market coordinate
across the capture axis, the episodes a declared selector found on it, and — for
whichever exact frame the playhead is parked on — the state the running binary
actually held there, navigable back to the raw bytes by identity.

Two rules shape the layout. Episode discovery never consults a SYMM output, so
what the desk did is drawn in its own band beside the market record rather than
inside it. And the future may choose where to look but may never change what was
known there, so nothing below the timeline is ever recomputed from what happened
afterwards.
*/

const COORDINATES: MarketCoordinate[] = ["midpoint", "trade", "last"];
const AXES: TimelineAxis[] = ["time", "capture"];
const DETAIL_BUCKETS = 320;
const OVERVIEW_BUCKETS = 200;

const digest = (value?: string | null): string =>
	value == null || value === "" ? "—" : value.slice(0, 10);

const HindsightRoute = () => {
	const [runs, setRuns] = useState<HindsightRun[]>([]);
	const [run, setRun] = useState<string | null>(null);
	const [coordinate, setCoordinate] = useState<MarketCoordinate>("midpoint");
	const [axis, setAxis] = useState<TimelineAxis>("time");
	const [symbol, setSymbol] = useState<string | null>(null);

	const [overview, setOverview] = useState<HindsightTimeline | null>(null);
	const [detail, setDetail] = useState<HindsightTimeline | null>(null);
	const [viewport, setViewport] = useState<{ from: number; to: number } | null>(
		null,
	);

	const [gaps, setGaps] = useState<HindsightGap[]>([]);
	const [lifecycle, setLifecycle] = useState<HindsightLifecycleEvent[]>([]);

	const [playhead, setPlayhead] = useState<HindsightRef | null>(null);
	const [episode, setEpisode] = useState<string | null>(null);
	const [captures, setCaptures] = useState<HindsightCapture[]>([]);
	const [envelope, setEnvelope] = useState<HindsightEnvelope | null>(null);
	const [state, setState] = useState<EnvelopeState | null>(null);
	const [loading, setLoading] = useState(false);
	const [semantics, setSemantics] = useState<HindsightMetricMap | null>(null);
	const [position, setPosition] = useState<string | null>(null);
	const [marks, setMarks] = useState<Mark[]>([]);
	const [markStates, setMarkStates] = useState<Array<EnvelopeState | null>>([]);
	const [residents, setResidents] = useState<Array<HindsightResident | null>>([]);
	const [compareMode, setCompareMode] = useState<CompareMode>("resident");
	const [resolving, setResolving] = useState(false);

	const surface = useRef<HTMLDivElement | null>(null);

	// The declared metric semantics are the same answer for every run and every
	// capture, so they are read once per session.
	useEffect(() => {
		let cancelled = false;

		fetchHindsightMetricMap().then((loaded) => {
			if (!cancelled) setSemantics(loaded);
		});

		return () => {
			cancelled = true;
		};
	}, []);

	useEffect(() => {
		let cancelled = false;

		fetchHindsightRuns().then((loaded) => {
			if (cancelled) return;

			setRuns(loaded);
			setRun((current) => current ?? loaded[0]?.id ?? null);
		});

		return () => {
			cancelled = true;
		};
	}, []);

	useEffect(() => {
		if (run === null) return;

		let cancelled = false;

		setSymbol(null);
		setViewport(null);
		setPlayhead(null);
		setEpisode(null);
		setEnvelope(null);
		setState(null);
		setCaptures([]);
		setMarks([]);
		setMarkStates([]);
		setResidents([]);
		setPosition(null);

		fetchHindsightGaps(run).then((loaded) => {
			if (!cancelled) setGaps(loaded);
		});

		fetchHindsightLifecycle(run).then((loaded) => {
			if (!cancelled) setLifecycle(loaded);
		});

		return () => {
			cancelled = true;
		};
	}, [run]);

	// The overview is the whole run for the selected instrument. It is also what
	// answers "which instrument?" on first load: with no symbol declared, the hub
	// projects the instrument whose declared coordinate travelled furthest.
	useEffect(() => {
		if (run === null) return;

		let cancelled = false;
		setLoading(true);

		fetchHindsightTimeline({
			run,
			symbol: symbol ?? undefined,
			coordinate,
			axis,
			buckets: OVERVIEW_BUCKETS,
			symbols: true,
		}).then((loaded) => {
			if (cancelled) return;

			setOverview(loaded);
			setLoading(false);

			if (loaded !== null && symbol === null && loaded.symbol !== "") {
				setSymbol(loaded.symbol);
			}
		});

		return () => {
			cancelled = true;
		};
	}, [run, symbol, coordinate, axis]);

	// The detail view is the same projection at the plotted window's resolution.
	// With no window it is the overview at full resolution.
	useEffect(() => {
		if (run === null || symbol === null) {
			setDetail(null);
			return;
		}

		let cancelled = false;

		fetchHindsightTimeline({
			run,
			symbol,
			coordinate,
			axis,
			buckets: DETAIL_BUCKETS,
			from: viewport?.from,
			to: viewport?.to,
		}).then((loaded) => {
			if (!cancelled) setDetail(loaded);
		});

		return () => {
			cancelled = true;
		};
	}, [run, symbol, coordinate, axis, viewport]);

	// Everything below the timeline is addressed by the playhead's exact capture
	// identity: the neighbouring raw frames, the envelopes this frame produced,
	// and the historical state witnessed at its observe boundary.
	useEffect(() => {
		if (run === null || playhead === null) return;

		let cancelled = false;
		const from = Math.max(playhead.sequence - 24, 0);

		fetchHindsightCaptures(run, from).then((loaded) => {
			if (!cancelled) setCaptures(loaded.slice(0, 48));
		});

		fetchHindsightEnvelope(run, playhead.sequence).then((loaded) => {
			if (!cancelled) setEnvelope(loaded);
		});

		fetchHindsightState(run, playhead.sequence, playhead.ordinal).then(
			(loaded) => {
				if (!cancelled) setState(decodeEnvelopeState(loaded?.payload));
			},
		);

		return () => {
			cancelled = true;
		};
	}, [run, playhead]);

	const positions = useMemo<Position[]>(
		() => buildPositions(lifecycle),
		[lifecycle],
	);

	/*
		Each mark's state is read by its own exact capture identity, never
		reconstructed from the neighbouring one. A mark whose envelope witnessed
		no state stays null, and the comparison reports it as unavailable rather
		than carrying the previous mark's values forward.
	*/
	useEffect(() => {
		if (run === null || marks.length === 0) {
			setMarkStates([]);
			return;
		}

		let cancelled = false;

		Promise.all(
			marks.map((mark) =>
				fetchHindsightState(run, mark.sequence, mark.ordinal).then((loaded) =>
					decodeEnvelopeState(loaded?.payload),
				),
			),
		).then((loaded) => {
			if (!cancelled) setMarkStates(loaded);
		});

		return () => {
			cancelled = true;
		};
	}, [run, marks]);

	/*
		Resident resolution is per (run, symbol, mark): the walk is over the
		instrument's own captures, so changing instrument changes the answer.
	*/
	useEffect(() => {
		if (run === null || symbol === null || marks.length === 0) {
			setResidents([]);
			return;
		}

		let cancelled = false;
		setResolving(true);

		Promise.all(
			marks.map((entry) =>
				fetchHindsightResident(run, symbol, entry.sequence, entry.ordinal),
			),
		).then((loaded) => {
			if (cancelled) return;

			setResidents(loaded);
			setResolving(false);
		});

		return () => {
			cancelled = true;
		};
	}, [run, symbol, marks]);

	const runMeta = useMemo(
		() => runs.find((entry) => entry.id === run) ?? null,
		[runs, run],
	);

	/*
		The capture card reads the identity the envelope read answered with. The
		surrounding frame strip is a listing, not the source of truth: a playhead
		parked outside the loaded neighbourhood must still name its own frame.
	*/
	const capture = useMemo(() => {
		if (
			envelope !== null &&
			envelope.capture?.identity?.sequence === playhead?.sequence
		) {
			return envelope.capture;
		}

		return (
			captures.find(
				(entry) => entry.identity.sequence === playhead?.sequence,
			) ?? null
		);
	}, [envelope, captures, playhead]);

	/*
		Reference points are the navigable targets of the whole surface: [ and ]
		step through them by exact HindsightRef (sequence, then ordinal), which
		is exactly the walk an inspection session wants — one interesting
		boundary to the next, never a scroll, and never a sequence-only hop that
		could leave the previous ordinal attached to a new sequence.
	*/
	const references = useMemo<HindsightRef[]>(() => {
		const points = (detail ?? overview)?.discovery.episodes.flatMap(
			(entry) => entry.references,
		);

		if (points === undefined) return [];

		return orderHindsightRefs(
			points.map((point) => ({
				sequence: point.capture.sequence,
				ordinal: point.ordinal,
			})),
		);
	}, [detail, overview]);

	const stepReference = useCallback(
		(direction: 1 | -1) => {
			if (references.length === 0) return;

			if (playhead === null) {
				setPlayhead(
					direction === 1 ? references[0] : references[references.length - 1],
				);

				return;
			}

			const next =
				direction === 1
					? references.find(
							(reference) => compareHindsightRef(reference, playhead) > 0,
						)
					: [...references]
							.reverse()
							.find(
								(reference) =>
									compareHindsightRef(reference, playhead) < 0,
							);

			if (next !== undefined) setPlayhead(next);
		},
		[references, playhead],
	);

	/*
		jumpRef parks the playhead on an exact causal EnvelopeRef. jumpChart
		parks on a raw sequence — a chart coordinate only — and resolves the
		first envelope (ordinal 0) of that capture deterministically rather than
		silently inheriting whatever ordinal a previous inspection had left.
	*/
	const jumpRef = useCallback((target: HindsightRef) => {
		setPlayhead({ sequence: target.sequence, ordinal: target.ordinal });
	}, []);

	const jumpChart = useCallback(
		(sequence: number) => {
			setPlayhead({ sequence, ordinal: 0 });
		},
		[],
	);

	const mark = useCallback(() => {
		if (playhead === null) return;

		setMarks((current) => {
			if (current.length >= 3) return current;
			if (
				current.some(
					(entry) =>
						entry.sequence === playhead.sequence &&
						entry.ordinal === playhead.ordinal,
				)
			) {
				return current;
			}

			return [
				...current,
				{
					sequence: playhead.sequence,
					ordinal: playhead.ordinal,
					label: `#${playhead.sequence}:${playhead.ordinal}`,
				},
			].sort((left, right) =>
				left.sequence !== right.sequence
					? left.sequence - right.sequence
					: left.ordinal - right.ordinal,
			);
		});
	}, [playhead]);

	/*
		Focusing a position parks the playhead on the nearest frame that really
		was captured around the venue's reported fill instant. That frame is an
		inspection location, not the fill's causal identity: the position's own
		identity stays its decision, and the parked ordinal is the resolved
		first envelope of the selected capture frame — never a previous
		inspection's ordinal silently carried onto a new sequence.
	*/
	const focusPosition = useCallback(
		(selected: Position) => {
			setPosition(selected.decisionId);

			const source = detail ?? overview;
			const at = new Date(selected.entry?.at ?? "").getTime();

			if (source === null || Number.isNaN(at)) return;

			let nearest: number | null = null;
			let distance = Number.POSITIVE_INFINITY;

			for (const bucket of source.buckets) {
				if (bucket.observations === 0) continue;

				const from = new Date(bucket.observedFromAt).getTime();

				if (Number.isNaN(from)) continue;

				const gap = Math.abs(from - at);

				if (gap < distance) {
					distance = gap;
					nearest = bucket.observedFromSequence;
				}
			}

			if (nearest !== null) setPlayhead({ sequence: nearest, ordinal: 0 });
		},
		[detail, overview],
	);

	const focusEpisode = useCallback((selected: HindsightEpisode) => {
		setEpisode(selected.id);

		const pad = Math.max(
			Math.round((selected.toSequence - selected.fromSequence) * 0.15),
			1,
		);

		setViewport({
			from: Math.max(selected.fromSequence - pad, 0),
			to: selected.toSequence + pad,
		});

		const anchor =
			selected.references.find((reference) => reference.role === "anchor") ??
			selected.references[0];

		// The anchor's ordinal is part of its causal identity and is preserved:
		// focusing an episode navigates to the exact reference, not a sequence.
		if (anchor !== undefined) {
			setPlayhead({
				sequence: anchor.capture.sequence,
				ordinal: anchor.ordinal,
			});
		}
	}, []);

	useEffect(() => {
		const onKey = (event: KeyboardEvent) => {
			if (event.target instanceof HTMLInputElement) return;

			switch (event.key) {
				case "[":
					stepReference(-1);
					break;
				case "]":
					stepReference(1);
					break;
				case "f":
					setViewport(null);
					break;
				case "m":
					mark();
					break;
				case "Escape":
					setViewport(null);
					setEpisode(null);
					setPosition(null);
					break;
				default:
					return;
			}

			event.preventDefault();
		};

		globalThis.addEventListener("keydown", onKey);

		return () => globalThis.removeEventListener("keydown", onKey);
	}, [stepReference, mark]);

	return (
		<div
			ref={surface}
			className="flex h-full min-w-275 flex-col overflow-hidden bg-(--bg)"
		>
			<RunBar
				runs={runs}
				run={run}
				runMeta={runMeta}
				overview={overview}
				coordinate={coordinate}
				axis={axis}
				gaps={gaps}
				onRun={setRun}
				onCoordinate={setCoordinate}
				onAxis={setAxis}
			/>

			<div className="flex min-h-0 flex-1 overflow-hidden">
				<div className="flex w-56 shrink-0 flex-col border-(--line) border-r">
					<SymbolTargets
						summaries={overview?.symbols ?? []}
						selected={symbol}
						onSelect={(next) => {
							setSymbol(next);
							setViewport(null);
							setEpisode(null);
						}}
					/>
				</div>

				<Flex.Column className="min-h-0 min-w-0 flex-1 overflow-hidden">
					<TimelineHeader
						timeline={detail ?? overview}
						window={viewport}
						loading={loading}
						references={references.length}
						marks={marks}
						playhead={playhead?.sequence ?? null}
						onMark={mark}
						onReset={() => {
							setViewport(null);
							setEpisode(null);
						}}
					/>

					<div className="shrink-0 border-(--line) border-b">
						<Timeline
							timeline={detail ?? overview}
							gaps={gaps}
							lifecycle={lifecycle}
							positions={positions}
							playhead={playhead?.sequence ?? null}
							marks={marks.map((entry) => entry.sequence)}
							selectedEpisode={episode}
							selectedPosition={position}
							onPlayhead={jumpChart}
							onEpisode={focusEpisode}
							onPosition={focusPosition}
							onZoom={(from, to) => setViewport({ from, to })}
							onResetZoom={() => setViewport(null)}
						/>
						<div className="flex items-center gap-2 border-(--line) border-t px-3 pt-1 font-mono text-[8px] text-(--f4) uppercase tracking-widest">
							<span>whole run</span>
							<span className="normal-case tracking-normal text-(--f4)">
								drag here to move the plotted window
							</span>
						</div>
						<Overview
							timeline={overview}
							window={
								viewport ??
								(detail === null
									? null
									: {
											from: detail.span.fromSequence,
											to: detail.span.toSequence,
										})
							}
							onZoom={(from, to) => setViewport({ from, to })}
							onResetZoom={() => setViewport(null)}
						/>
					</div>

					<div className="flex min-h-0 flex-1 overflow-hidden">
						<div className="flex w-80 shrink-0 flex-col border-(--line) border-r">
							<EpisodeTargets
								timeline={detail ?? overview}
								selected={episode}
								onSelect={focusEpisode}
								onReference={jumpRef}
							/>
						</div>

						<Flex.Column className="min-h-0 min-w-0 flex-1 overflow-hidden">
							{marks.length >= 2 ? (
								<ComparePanel
									marks={marks}
									states={markStates}
									residents={residents}
									mode={compareMode}
									loading={resolving}
									onMode={setCompareMode}
									onPlayhead={(sequence, ordinal) => {
										setPlayhead({ sequence, ordinal });
									}}
									onClear={() => setMarks([])}
									onRemove={(sequence, ordinal) =>
										setMarks((current) =>
											current.filter(
												(entry) =>
													entry.sequence !== sequence || entry.ordinal !== ordinal,
											),
										)
									}
								/>
							) : playhead === null ? (
								<p className="px-3 py-8 font-mono text-[10px] text-(--f4) leading-relaxed">
									Pick an episode, or click the timeline, to park the playhead on an
									exact captured frame.
									<br />
									Everything here is then read from that identity — the raw frame, the
									envelopes it produced, and the state the running binary actually held.
									Never from a nearby timestamp.
								</p>
							) : (
								<>
									<CaptureCard capture={capture} run={runMeta} />
									<FrameStrip
										captures={captures}
										playhead={playhead?.sequence ?? null}
										onSelect={jumpChart}
									/>
									<div className="min-h-0 flex-1 overflow-auto">
										<div className="grid grid-cols-2">
											<div className="min-w-0 border-(--line) border-r">
												<ProvenancePanel
													envelope={envelope}
													onSelect={(sequence, ordinal) =>
														jumpRef({ sequence, ordinal })
													}
												/>
											</div>
											<div className="min-w-0">
												<StatePanel
												state={state}
												envelope={envelope}
												semantics={semantics}
											/>
											</div>
										</div>
									</div>
								</>
							)}
						</Flex.Column>
					</div>
				</Flex.Column>
			</div>
		</div>
	);
};

const RunBar = ({
	runs,
	run,
	runMeta,
	overview,
	coordinate,
	axis,
	gaps,
	onRun,
	onCoordinate,
	onAxis,
}: {
	runs: HindsightRun[];
	run: string | null;
	runMeta: HindsightRun | null;
	overview: HindsightTimeline | null;
	coordinate: MarketCoordinate;
	axis: TimelineAxis;
	gaps: HindsightGap[];
	onRun: (id: string) => void;
	onCoordinate: (next: MarketCoordinate) => void;
	onAxis: (next: TimelineAxis) => void;
}) => (
	<Flex.Column className="shrink-0 border-(--line) border-b bg-(--surface)">
		<Flex.Row
			align="center"
			gap={3}
			className="h-10 shrink-0 overflow-x-auto px-3"
		>
			<span className="shrink-0 font-mono text-[8px] text-(--f4) uppercase tracking-widest">
				run
			</span>
			{runs.length === 0 ? (
				<span className="font-mono text-[10px] text-(--f4)">
					No capture run recorded yet.
				</span>
			) : null}
			{runs.map((entry) => {
				const active = entry.id === run;

				return (
					<Button
						key={entry.id}
						variant="bare"
						title={`${entry.id}\ncommit ${entry.codeCommit || "—"} · build ${entry.buildId || "—"} · config ${entry.configDigest || "—"}`}
						className={`shrink-0 rounded-[3px] border px-2 py-1 font-mono text-[9px] ${
							active
								? "border-(--acc) bg-(--raised) text-(--f1)"
								: "border-(--line) text-(--f4) hover:border-(--line2) hover:text-(--f2)"
						}`}
						onClick={() => onRun(entry.id)}
					>
						{new Date(entry.startedAt).toLocaleString([], {
							month: "short",
							day: "2-digit",
							hour: "2-digit",
							minute: "2-digit",
						})}
						<span
							className={`ml-1.5 ${entry.integrity === "COMPLETE" ? "text-(--up)" : "text-(--warn)"}`}
						>
							{entry.integrity === "COMPLETE" ? "●" : "◐"}
						</span>
					</Button>
				);
			})}

			<span className="ml-auto shrink-0" />

			<Flex.Row align="center" gap={1} className="shrink-0">
				<span className="font-mono text-[8px] text-(--f4) uppercase tracking-widest">
					coordinate
				</span>
				{COORDINATES.map((option) => (
					<Button
						key={option}
						variant="bare"
						title={`Declare the market coordinate the selector measures. "${option}" means exactly that quantity — never a realisable price.`}
						className={`rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] ${
							coordinate === option
								? "border-(--acc) text-(--f1)"
								: "border-(--line) text-(--f4) hover:text-(--f2)"
						}`}
						onClick={() => onCoordinate(option)}
					>
						{option}
					</Button>
				))}
			</Flex.Row>

			<Flex.Row align="center" gap={1} className="shrink-0">
				<span className="font-mono text-[8px] text-(--f4) uppercase tracking-widest">
					axis
				</span>
				{AXES.map((option) => (
					<Button
						key={option}
						variant="bare"
						title={
							option === "time"
								? "Position by wall clock. Identity stays capture sequence."
								: "Position by capture sequence — the order SYMM observed the world in."
						}
						className={`rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] ${
							axis === option
								? "border-(--acc) text-(--f1)"
								: "border-(--line) text-(--f4) hover:text-(--f2)"
						}`}
						onClick={() => onAxis(option)}
					>
						{option}
					</Button>
				))}
			</Flex.Row>
		</Flex.Row>

		<Flex.Row
			align="center"
			gap={4}
			className="h-7 shrink-0 flex-wrap border-(--line) border-t px-3 font-mono text-[9px] text-(--f4)"
		>
			<span>
				commit <span className="text-(--f2)">{digest(runMeta?.codeCommit)}</span>
			</span>
			<span>
				build <span className="text-(--f2)">{digest(runMeta?.buildId)}</span>
			</span>
			<span>
				config <span className="text-(--f2)">{digest(runMeta?.configDigest)}</span>
			</span>
			<span>
				captured{" "}
				<span className="text-(--f2)">
					{formatClock(overview?.runSpan.fromAt ?? "")} →{" "}
					{formatClock(overview?.runSpan.toAt ?? "")}
				</span>
			</span>
			<span>
				observations{" "}
				<span className="text-(--f2) tabular-nums">
					{formatCount(overview?.totalObservations ?? 0)}
				</span>{" "}
				over{" "}
				<span className="text-(--f2) tabular-nums">
					{formatCount(overview?.totalSymbols ?? 0)}
				</span>{" "}
				instruments
			</span>
			{gaps.length > 0 ? (
				<span
					className="text-(--down)"
					title={gaps
						.slice(0, 8)
						.map((gap) => `${gap.encoding} @ ${gap.sequence}: ${gap.detail}`)
						.join("\n")}
				>
					{gaps.length} capture integrity defect{gaps.length === 1 ? "" : "s"} —
					inspection certainty is broken across them
				</span>
			) : (
				<span className="text-(--up)">no capture integrity defect recorded</span>
			)}
		</Flex.Row>
	</Flex.Column>
);

const TimelineHeader = ({
	timeline,
	window,
	loading,
	references,
	marks,
	playhead,
	onMark,
	onReset,
}: {
	timeline: HindsightTimeline | null;
	window: { from: number; to: number } | null;
	loading: boolean;
	references: number;
	marks: Mark[];
	playhead: number | null;
	onMark: () => void;
	onReset: () => void;
}) => (
	<Flex.Row
		align="center"
		gap={4}
		className="h-9 shrink-0 border-(--line) border-b bg-(--surface) px-3"
	>
		<span className="font-mono text-[12px] font-semibold text-(--f1)">
			{timeline?.symbol || "—"}
		</span>
		<span className="font-mono text-[9px] text-(--f4)">
			observed {timeline?.coordinate ?? "—"} across{" "}
			<span className="text-(--f2) tabular-nums">
				{formatCount(timeline?.discovery.defined ?? 0)}
			</span>{" "}
			defined observations
		</span>
		<span className="font-mono text-[9px] text-(--f4)">
			capture{" "}
			<span className="text-(--f2) tabular-nums">
				{timeline?.span.fromSequence ?? 0}–{timeline?.span.toSequence ?? 0}
			</span>
		</span>
		<span className="font-mono text-[9px] text-(--f4)">
			<span className="text-(--f2) tabular-nums">{references}</span> reference
			points ·{" "}
			<span className="rounded-[2px] border border-(--line2) px-1">[</span>{" "}
			<span className="rounded-[2px] border border-(--line2) px-1">]</span> to walk
			them
		</span>

		<span className="ml-auto" />

		<MarkBar marks={marks} playhead={playhead} onMark={onMark} />

		{loading ? (
			<span className="font-mono text-[9px] text-(--acc)">indexing tape…</span>
		) : null}

		{window !== null ? (
			<Button
				variant="outline"
				size="xs"
				className="font-mono text-[9px]"
				onClick={onReset}
			>
				fit run (f)
			</Button>
		) : null}
	</Flex.Row>
);

export const Route = createFileRoute("/hindsight")({
	component: HindsightRoute,
});
