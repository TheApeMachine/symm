import { createFileRoute } from "@tanstack/react-router";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
	type CompareMode,
	ComparePanel,
	type Mark,
	MarkBar,
} from "#/components/hindsight/compare";
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
	buildPositions,
	type Position,
} from "#/components/hindsight/positions";
import { PositionIndex } from "#/components/hindsight/position-index";
import { Guide } from "#/components/hindsight/guide";
import { EpisodeTargets, SymbolTargets } from "#/components/hindsight/targets";
import { Overview, Timeline } from "#/components/hindsight/timeline";
import {
	formatClock,
	formatCount,
} from "#/components/hindsight/timeline-scale";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import type { EnvelopeState } from "#/providers/telemetry/telemetry/envelope-state";

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

/*
The reader's working posture, remembered between sessions. A split is the
fraction of the inspection row given to provenance, held away from either edge
so a pane can never be dragged out of existence.
*/
const SPLIT_KEY = "hindsight.split";
const PLAIN_KEY = "hindsight.plain";
const MIN_SPLIT = 0.15;
const MAX_SPLIT = 0.85;

const storedSplit = (): number => {
	const stored = Number(globalThis.localStorage?.getItem(SPLIT_KEY));

	if (!Number.isFinite(stored) || stored < MIN_SPLIT || stored > MAX_SPLIT) {
		return 0.5;
	}

	return stored;
};

const INITIAL_SPLIT = storedSplit();
const INITIAL_PLAIN = globalThis.localStorage?.getItem(PLAIN_KEY) !== "0";

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
	const [resident, setResident] = useState<HindsightResident | null>(null);
	const [loading, setLoading] = useState(false);
	const [semantics, setSemantics] = useState<HindsightMetricMap | null>(null);
	const [position, setPosition] = useState<string | null>(null);
	const [marks, setMarks] = useState<Mark[]>([]);
	const [markStates, setMarkStates] = useState<Array<EnvelopeState | null>>([]);
	const [residents, setResidents] = useState<Array<HindsightResident | null>>(
		[],
	);
	const [compareMode, setCompareMode] = useState<CompareMode>("resident");
	const [resolving, setResolving] = useState(false);

	/*
		Layout is the reader's, not the surface's. Inspection starts as a band
		under the market record, but once a finding is being read the evidence is
		the whole job — so the inspector can take the surface, and the split
		between provenance and state can be dragged. Both choices are the
		reader's working posture rather than a property of the run, so they are
		remembered across runs and reloads.
	*/
	const [focus, setFocus] = useState(false);
	const [guide, setGuide] = useState(false);
	const [split, setSplit] = useState(INITIAL_SPLIT);
	const [plain, setPlain] = useState(INITIAL_PLAIN);

	useEffect(() => {
		globalThis.localStorage?.setItem(SPLIT_KEY, String(split));
	}, [split]);

	useEffect(() => {
		globalThis.localStorage?.setItem(PLAIN_KEY, plain ? "1" : "0");
	}, [plain]);

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
		setResident(null);
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
		setState(null);
		setResident(null);

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

		if (symbol !== null) {
			fetchHindsightResident(
				run,
				symbol,
				playhead.sequence,
				playhead.ordinal,
			).then((loaded) => {
				if (!cancelled) setResident(loaded);
			});
		}

		return () => {
			cancelled = true;
		};
	}, [run, symbol, playhead]);

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
								(reference) => compareHindsightRef(reference, playhead) < 0,
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

	const jumpChart = useCallback((sequence: number) => {
		setPlayhead({ sequence, ordinal: 0 });
	}, []);

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
	/*
		focusPosition parks the tape on the position's own opening frame.

		The store resolves that frame exactly, by joining the lifecycle record
		to the decision witness that caused it, so the seek lands on the capture
		the desk actually decided on. The nearest-bucket search below is the
		fallback for records written before that correlation existed: it lands
		on whichever bucket started closest in wall time, which is an
		approximation and is only used when there is no recorded frame.
	*/
	const focusPosition = useCallback(
		(selected: Position) => {
			setPosition(selected.decisionId);

			if (selected.symbol !== "") setSymbol(selected.symbol);

			if (selected.entrySeq !== null) {
				setViewport(null);
				setEpisode(null);
				setPlayhead({ sequence: selected.entrySeq, ordinal: 0 });

				return;
			}

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

	/*
		seekPosition parks the tape on one edge of a position. Only an edge the
		record actually named is offered, so this never has to guess which frame
		an unstamped exit belonged to.
	*/
	const seekPosition = useCallback(
		(selected: Position, edge: "entry" | "exit") => {
			const sequence =
				edge === "entry" ? selected.entrySeq : selected.exitSeq;

			if (sequence === null) return;

			setPosition(selected.decisionId);

			if (selected.symbol !== "") setSymbol(selected.symbol);

			setViewport(null);
			setEpisode(null);
			setPlayhead({ sequence, ordinal: 0 });
		},
		[],
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
				case "e":
					setFocus((current) => !current);
					break;
				case "?":
					setPlain((current) => !current);
					break;
				case "h":
					setGuide((current) => !current);
					break;
				case "Escape":
					if (guide) {
						setGuide(false);
						break;
					}

					if (focus) {
						setFocus(false);
						break;
					}

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
	}, [stepReference, mark, focus, guide]);

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

			{guide ? <Guide onClose={() => setGuide(false)} /> : null}

			<div
				className={`${guide ? "hidden" : "flex"} min-h-0 flex-1 overflow-hidden`}
			>
				<div
					className={`${focus ? "hidden" : "flex"} w-56 shrink-0 flex-col border-(--line) border-r`}
				>
					<PositionIndex
						positions={positions}
						selected={position}
						onSelect={focusPosition}
						onSeek={seekPosition}
					/>

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
						focus={focus}
						plain={plain}
						onGuide={() => setGuide((current) => !current)}
						onFocus={() => setFocus((current) => !current)}
						onPlain={() => setPlain((current) => !current)}
						onMark={mark}
						onReset={() => {
							setViewport(null);
							setEpisode(null);
						}}
					/>

					<div
						className={`${focus ? "hidden" : "block"} shrink-0 border-(--line) border-b`}
					>
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
						<div
							className={`${focus ? "hidden" : "flex"} w-80 shrink-0 flex-col border-(--line) border-r`}
						>
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
													entry.sequence !== sequence ||
													entry.ordinal !== ordinal,
											),
										)
									}
								/>
							) : playhead === null ? (
								<p className="px-3 py-8 font-mono text-[10px] text-(--f4) leading-relaxed">
									Pick an episode, or click the timeline, to park the playhead
									on an exact captured frame.
									<br />
									Everything here is then read from that identity — the raw
									frame, the envelopes it produced, and the state the running
									binary actually held. Never from a nearby timestamp.
								</p>
							) : (
								<>
									<CaptureCard capture={capture} run={runMeta} />
									<FrameStrip
										captures={captures}
										playhead={playhead?.sequence ?? null}
										onSelect={jumpChart}
									/>
									<InspectionRow
										split={split}
										onSplit={setSplit}
										provenance={
											<ProvenancePanel
												envelope={envelope}
												onSelect={(sequence, ordinal) =>
													jumpRef({ sequence, ordinal })
												}
											/>
										}
										state={
											<StatePanel
												state={state}
												resident={resident}
												envelope={envelope}
												semantics={semantics}
												plain={plain}
											/>
										}
									/>
								</>
							)}
						</Flex.Column>
					</div>
				</Flex.Column>
			</div>
		</div>
	);
};

/*
InspectionRow is the evidence row: provenance on the left, the state the binary
held on the right, with the boundary between them owned by the reader.

The split is dragged rather than stepped because the two panes are read at very
different widths depending on the question — walking a causal chain wants the
left, reading a metric table wants the right — and no fixed ratio serves both.
The handle is also a real focusable control, so the split can be moved without
a pointer.
*/
const InspectionRow = ({
	split,
	onSplit,
	provenance,
	state,
}: {
	split: number;
	onSplit: (next: number) => void;
	provenance: React.ReactNode;
	state: React.ReactNode;
}) => {
	const row = useRef<HTMLDivElement | null>(null);

	const drag = useCallback(
		(event: React.PointerEvent<HTMLDivElement>) => {
			event.preventDefault();

			const bounds = row.current?.getBoundingClientRect();

			if (bounds === undefined || bounds.width === 0) return;

			const move = (moved: PointerEvent) => {
				const fraction = (moved.clientX - bounds.left) / bounds.width;

				onSplit(Math.min(Math.max(fraction, MIN_SPLIT), MAX_SPLIT));
			};

			const release = () => {
				globalThis.removeEventListener("pointermove", move);
				globalThis.removeEventListener("pointerup", release);
			};

			globalThis.addEventListener("pointermove", move);
			globalThis.addEventListener("pointerup", release);
		},
		[onSplit],
	);

	return (
		<div ref={row} className="flex min-h-0 flex-1 overflow-hidden">
			<div
				className="min-w-0 overflow-auto"
				style={{ flexBasis: `${split * 100}%` }}
			>
				{provenance}
			</div>

			<div
				role="slider"
				aria-label="Resize the evidence panes"
				aria-orientation="vertical"
				aria-valuemin={Math.round(MIN_SPLIT * 100)}
				aria-valuemax={Math.round(MAX_SPLIT * 100)}
				aria-valuenow={Math.round(split * 100)}
				tabIndex={0}
				title="Drag to rebalance · ← → to nudge · double-click to even up"
				className="w-1 shrink-0 cursor-col-resize border-(--line) border-x bg-(--line) hover:bg-(--acc) focus:bg-(--acc) focus:outline-none"
				onPointerDown={drag}
				onDoubleClick={() => onSplit(0.5)}
				onKeyDown={(event) => {
					if (event.key === "ArrowLeft") {
						onSplit(Math.max(split - 0.05, MIN_SPLIT));
					}

					if (event.key === "ArrowRight") {
						onSplit(Math.min(split + 0.05, MAX_SPLIT));
					}
				}}
			/>

			<div className="min-w-0 flex-1 overflow-auto">{state}</div>
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
						title={`${entry.id}\ncommit ${entry.codeCommit || "—"} · build ${entry.buildId || "—"} · config ${entry.configDigest || "—"}\npositions held: ${entry.positions ?? 0}`}
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
						{entry.positions ? (
							<span className="ml-1.5 text-(--acc)">{entry.positions}▲</span>
						) : null}
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
				commit{" "}
				<span className="text-(--f2)">{digest(runMeta?.codeCommit)}</span>
			</span>
			<span>
				build <span className="text-(--f2)">{digest(runMeta?.buildId)}</span>
			</span>
			<span>
				config{" "}
				<span className="text-(--f2)">{digest(runMeta?.configDigest)}</span>
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
				<span className="text-(--up)">
					no capture integrity defect recorded
				</span>
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
	focus,
	plain,
	onGuide,
	onFocus,
	onPlain,
	onMark,
	onReset,
}: {
	timeline: HindsightTimeline | null;
	window: { from: number; to: number } | null;
	loading: boolean;
	references: number;
	marks: Mark[];
	playhead: number | null;
	focus: boolean;
	plain: boolean;
	onGuide: () => void;
	onFocus: () => void;
	onPlain: () => void;
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
			<span className="rounded-xs border border-(--line2) px-1">[</span>{" "}
			<span className="rounded-xs border border-(--line2) px-1">]</span> to walk
			them
		</span>

		<span className="ml-auto" />

		<Button
			variant="bare"
			title="What every band of this surface is, and how to read it. (h)"
			className="rounded-[3px] border border-(--line) px-1.5 py-0.5 font-mono text-[9px] text-(--f4) hover:text-(--f2)"
			onClick={onGuide}
		>
			how to read this (h)
		</Button>

		<Button
			variant="bare"
			title={
				plain
					? "Reading in plain language. Switch to the system's own vocabulary. (?)"
					: "Reading in the system's own vocabulary. Switch to plain language. (?)"
			}
			className={`rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] ${
				plain
					? "border-(--acc) text-(--f1)"
					: "border-(--line) text-(--f4) hover:text-(--f2)"
			}`}
			onClick={onPlain}
		>
			{plain ? "plain" : "expert"}
		</Button>

		<Button
			variant="bare"
			title={
				focus
					? "Bring the market record back. (e / Esc)"
					: "Give the whole surface to the evidence. (e)"
			}
			className={`rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] ${
				focus
					? "border-(--acc) text-(--f1)"
					: "border-(--line) text-(--f4) hover:text-(--f2)"
			}`}
			onClick={onFocus}
		>
			{focus ? "exit focus (e)" : "focus (e)"}
		</Button>

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
