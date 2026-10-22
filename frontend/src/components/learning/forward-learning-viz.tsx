import { useSelector } from "@tanstack/react-store";
import * as d3 from "d3";
import { AnimatePresence, motion } from "motion/react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
	atoms,
	signals,
} from "#/collections/app";
import {
	branchesFromPolicyKeys,
	cognitionStore,
} from "#/collections/cognition";
import {
	abortDetections,
	beginDetectionRuns,
	beginDetections,
	beginDetectionTape,
	type Detection,
	type DetectionRun,
	detectionsStore,
} from "#/collections/detections";
import { Scanlines } from "#/components/ui/scanlines";
import { cn } from "#/lib/utils";
import { sendCommand } from "#/providers/websocket";
import type { Value } from "#/types/value";
import { Alert, Badge, Button, Flex, Section, Typography } from "../ui";
import { basis, outcome, prediction } from "./format";

interface ForwardTapePoint {
	x: number;
	y: number;
	seq?: number;
	time?: number;
}

interface TrainedFragmentResponse {
	id: number;
	symbol: string;
	/* The tick epoch detect labelled (a decimal string: beyond a JS number). */
	epoch: string;
	/* The excursion's C tick, which names it in its run (decimal string). */
	c_tick: string;
	mark_a: number;
	mark_b: number;
	mark_c: number;
	entry_price: number;
	exit_price: number;
	magnitude: number;
	direction: string;
	class?: string;
	tokens: string[];
	points: ForwardTapePoint[];
	entry_idx: number;
	exit_idx: number;
	predicted_entry_idx: number;
	predicted_exit_idx: number;
	learned_at: string;
}

/*
fragmentOf maps one stored excursion (detect's detector/<class>/<symbol>/
<epoch>/<c tick>.bin) onto the fragment this view draws. Marks, entry and
exit are detect's trade indices (a_idx..), which the tape's points carry as
seq. Excursions carry no precursor tokens and no predicted entry/exit (those
are train's and paper's), so tokens are empty and the predictions -1; the
time shown is C's (when the move was exhausted).
*/
const fragmentOf = (
	run: string,
	detection: Detection,
	idx: number,
): TrainedFragmentResponse => {
	const fields = detection.fields;
	return {
		id: idx + 1,
		symbol: detection.symbol,
		epoch: run,
		c_tick: String(fields.c_tick),
		mark_a: fields.a_idx,
		mark_b: fields.b_idx,
		mark_c: fields.c_idx,
		entry_price: fields.entry_ask,
		exit_price: fields.exit_bid,
		magnitude:
			fields.b_price > 0
				? (fields.c_price - fields.b_price) / fields.b_price
				: 0,
		direction: detection.class,
		class: detection.class,
		tokens: [],
		points: [],
		entry_idx: fields.entry_idx,
		exit_idx: fields.exit_idx,
		predicted_entry_idx: -1,
		predicted_exit_idx: -1,
		learned_at: new Date(fields.c_at / 1e6).toISOString(),
	};
};

const NO_RUNS: DetectionRun[] = [];

function parseTimestamp(
	atNs: bigint | number | undefined | null,
	obsNs?: bigint | number | null,
): number {
	const val = atNs && atNs > 0n ? atNs : obsNs && obsNs > 0n ? obsNs : null;
	if (val) {
		const num = typeof val === "bigint" ? Number(val) : val;
		if (num > 1e16) return Math.floor(num / 1e6); // nanoseconds
		if (num > 1e13) return Math.floor(num / 1e3); // microseconds
		if (num > 1e10) return Math.floor(num); // milliseconds
		if (num > 1e6) return Math.floor(num * 1000); // seconds
	}
	return Date.now();
}

function toTimestampMs(time: number | undefined | null): number | undefined {
	if (!time || time <= 0) return undefined;
	if (time > 1e16) return Math.floor(time / 1e6); // nanoseconds -> ms
	if (time > 1e13) return Math.floor(time / 1e3); // microseconds -> ms
	if (time > 1e10) return Math.floor(time); // milliseconds
	if (time > 1e6) return Math.floor(time * 1000); // seconds -> ms
	return undefined;
}

function formatDurationSpan(ms: number): string {
	if (!Number.isFinite(ms) || ms <= 0) return "";
	if (ms < 1000) return `${(ms / 1000).toFixed(2)}s`;
	const totalSec = Math.round(ms / 1000);
	if (totalSec < 60) return `${totalSec}s`;
	const mins = Math.floor(totalSec / 60);
	const secs = totalSec % 60;
	if (mins < 60) {
		return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`;
	}
	const hrs = Math.floor(mins / 60);
	const remMins = mins % 60;
	return remMins > 0 ? `${hrs}h ${remMins}m` : `${hrs}h`;
}

function formatClockTime(ms: number, withSeconds: boolean): string {
	const d = new Date(ms);
	if (Number.isNaN(d.getTime())) return "—";
	return d.toLocaleTimeString([], {
		hour: "2-digit",
		minute: "2-digit",
		second: withSeconds ? "2-digit" : undefined,
		hour12: false,
	});
}

type ExcursionKind =
	| "UPWARD EXCURSION"
	| "UP FRICTION"
	| "DOWNWARD EXCURSION"
	| "CHOPPY MARKET"
	| "FLAT TAPE";

function excursionKind(
	direction: string,
	typeCode: number,
): ExcursionKind | null {
	const named = outcome(direction, typeCode);
	if (named === "UP") return "UPWARD EXCURSION";
	if (named === "UP_FRICTION") return "UP FRICTION";
	if (named === "DOWN") return "DOWNWARD EXCURSION";
	if (named === "CHOP") return "CHOPPY MARKET";
	if (named === "FLAT") return "FLAT TAPE";
	return null;
}

/* fragmentClass maps the five trained tape classes onto the banner label. */
function fragmentClass(
	className: string | undefined,
	direction: string,
): ExcursionKind {
	switch (className) {
		case "up":
			return "UPWARD EXCURSION";
		case "up_friction":
			return "UP FRICTION";
		case "down":
			return "DOWNWARD EXCURSION";
		case "chop":
			return "CHOPPY MARKET";
		case "flat":
			return "FLAT TAPE";
		default:
			return direction === "up"
				? "UPWARD EXCURSION"
				: direction === "down"
					? "DOWNWARD EXCURSION"
					: direction === "chop"
						? "CHOPPY MARKET"
						: "FLAT TAPE";
	}
}

/* fragmentClassCode keeps the raw class token for the operator list/detail. */
function fragmentClassCode(
	className: string | undefined,
	direction: string,
): string {
	if (className) return className;
	return direction || "unknown";
}

function fragmentDelayedLabel(
	className: string | undefined,
	direction: string,
): string {
	switch (className ?? direction) {
		case "up":
			return "ENTER (CLEARS)";
		case "up_friction":
			return "WAIT (FRICTION)";
		case "down":
			return "WAIT (DOWN)";
		case "chop":
			return "WAIT (CHOP)";
		case "flat":
			return "WAIT (FLAT)";
		default:
			return "RESOLVING";
	}
}

function fragmentFrozenAction(
	className: string | undefined,
	direction: string,
): string {
	return (className ?? direction) === "up" ? "ENTER" : "WAIT";
}

function outcomeText(
	direction: string,
	typeCode: number,
	target: number | undefined,
): string {
	const named = outcome(direction, typeCode);
	if (named) return named;
	if (target === 1) return "ENTER (CLEARS)";
	if (target === 2) return "EXIT";
	if (target !== undefined) return "ABSTAIN";
	return "RESOLVING";
}

function signedPercent(fraction: number): string {
	const pct = fraction * 100;
	const body = pct.toFixed(2);
	if (pct > 0) return `+${body}%`;
	return `${body}%`;
}

const getTargetFrame = (
	records: Record<string, Value> | undefined,
	target: string,
): { frame: Value; symbol: string } | null => {
	if (!records) return null;
	if (target && records[target]) {
		return { frame: records[target], symbol: target };
	}
	if (records.learner) {
		return { frame: records.learner, symbol: "learner" };
	}
	let best: Value | null = null;
	let bestSym = "";
	let latestTick = -1n;

	for (const [sym, frame] of Object.entries(records)) {
		if (!frame) continue;
		if (frame.tick >= latestTick || best === null) {
			latestTick = frame.tick;
			best = frame;
			bestSym = sym;
		}
	}
	if (best) {
		return { frame: best, symbol: bestSym };
	}
	return null;
};

const ListRun = ({
	active,
	onClick,
	label,
	mark,
	detail,
	value,
}: {
	active: boolean;
	onClick: () => void;
	label: string;
	mark: string;
	detail: string;
	value: string;
}) => (
	<Button
		variant={active ? "solid" : "quiet"}
		tone={active ? "accent" : "muted"}
		shape="block"
		fullWidth
		size="xs"
		onClick={onClick}
		data-l={active ? "historical-run-active" : undefined}
	>
		<Flex.Column fullWidth>
			<Flex.Row justify="between" align="center" fullWidth>
				<Typography.Mono size="s" tone={active ? "accent" : "f1"} weight="semibold">
					{label}
				</Typography.Mono>
				<Badge size="xs" variant={active ? "info" : "disabled"} label={mark} />
			</Flex.Row>
			<Flex.Row justify="between" fullWidth>
				<Typography.Mono size="xs" tone="f4">
					{detail}
				</Typography.Mono>
				<Typography.Mono size="xs" tone="f4">
					{value}
				</Typography.Mono>
			</Flex.Row>
		</Flex.Column>
	</Button>
);

export interface ForwardLearningVizProps {
	symbol?: string;
	/*
	 * live (default): MODEL TRAINING — stage-1 stream paints the tape, but a
	 * selected fragment pins the chart so BTC/USD hub updates cannot stomp it.
	 * historical: HISTORICAL RUNS tab — tape only from /training/fragments;
	 * live hub never writes points/symbol/excursion on the chart.
	 */
	tapeSource?: "live" | "historical";
}

export const ForwardLearningViz = ({
	symbol: propSymbol,
	tapeSource = "live",
}: ForwardLearningVizProps = {}) => {
	const globalFocus = useSelector(atoms.focus, (state) => state);
	const targetSymbol = propSymbol || globalFocus;
	const historicalOnly = tapeSource === "historical";
	const tapeRef = useRef<HTMLDivElement>(null);
	const [tapeDim, setTapeDim] = useState({ width: 800, height: 300 });
	const [isPlaying, setIsPlaying] = useState(true);

	// Real tape points: live stage stream and/or a pinned trained fragment.
	const [points, setPoints] = useState<ForwardTapePoint[]>([]);

	// Real stage & execution mode
	const [stageCode, setStageCode] = useState(0);
	const [stageBlocker, setStageBlocker] = useState("");
	const [openPositionsCount, setOpenPositionsCount] = useState(0);
	const [isPaperFilled, setIsPaperFilled] = useState(false);

	// Precursor & prediction
	const [precursorLength, setPrecursorLength] = useState(0);
	const [rawPrecursorTokens, setRawPrecursorTokens] = useState<string[]>([]);
	const [frozenAction, setFrozenAction] = useState("ABSTAIN");
	const [delayedLabel, setDelayedLabel] = useState("RESOLVING");

	// Historical held-out metrics
	const [histOpportunities, setHistOpportunities] = useState(0);
	const [histCorrectEnter, setHistCorrectEnter] = useState(0);
	const [histMissedEnter, setHistMissedEnter] = useState(0);
	const [histFalseEnter, setHistFalseEnter] = useState(0);
	const [histMeanReturn, setHistMeanReturn] = useState(0);
	const [histLowerBound, setHistLowerBound] = useState(0);

	// Forward paper metrics
	const [fwdPredictions, setFwdPredictions] = useState(0);
	const [fwdPaperTrades, setFwdPaperTrades] = useState(0);
	const [fwdPaperMeanReturn, setFwdPaperMeanReturn] = useState(0);
	const [fwdPaperLowerBound, setFwdPaperLowerBound] = useState(0);

	// Overall counters
	const [evaluatedCount, setEvaluatedCount] = useState(0);
	// Observed skill return samples (+1/-1 from recordSkill) for the edge panel.
	const [edgeSamples, setEdgeSamples] = useState<number[]>([]);

	// Real Radix Trie branches fetched from backend engine
	// The loaded CognitionTree's stored branches (the hub's cognition/tree frame).
	const trieBranches = useSelector(cognitionStore, (state) => {
		if (state.tree?.branches?.length) return state.tree.branches;
		const keys =
			state.policyKeys.length > 0 ? state.policyKeys : (state.tree?.keys ?? []);
		return branchesFromPolicyKeys(keys);
	});

	// Real excursion markers if active
	const [excursionEvent, setExcursionEvent] = useState<{
		type: ExcursionKind | null;
		magnitude: number;
		marks: { A: number; B: number; C: number };
		entryIdx: number | null;
		exitIdx: number | null;
		predictedEntryIdx: number | null;
		predictedExitIdx: number | null;
		classCode?: string;
	} | null>(null);

	// Episode Buffer for playback
	const [episodeQueue, setEpisodeQueue] = useState<ForwardTapePoint[][]>([]);
	const [currentEpisode, setCurrentEpisode] = useState<
		ForwardTapePoint[] | null
	>(null);
	const [playbackTick, setPlaybackTick] = useState(0);
	const [playbackPhase, setPlaybackPhase] = useState<"PLAYING" | "EVALUATING">(
		"PLAYING",
	);
	const pendingEpisodeRef = useRef<ForwardTapePoint[]>([]);
	const currentEpisodeRef = useRef<ForwardTapePoint[] | null>(null);

	// Current training symbol (not filtered by user focus atom — reflects whatever is training)
	const [currentSymbol, setCurrentSymbol] = useState("TRAINING");
	const lastSeenSymbolRef = useRef<string>("");
	const lastSeenExcursionStartRef = useRef<number>(-1);

	const [trainedFragments, setTrainedFragments] = useState<
		TrainedFragmentResponse[]
	>([]);
	const [selectedFragmentIndex, setSelectedFragmentIndex] =
		useState<number>(-1);
	/* The tick epoch whose detections are listed, and why a request failed. */
	const [detectionRun, setDetectionRun] = useState<string | null>(null);
	const [detectionsError, setDetectionsError] = useState<string | null>(null);
	/*
	 * pinnedFragmentRef freezes the tape chart while the operator inspects a
	 * historical run. Live hub measurements still update stage/side metrics,
	 * but must not rewrite points, symbol, or excursion markers on the chart.
	 */
	const pinnedFragmentRef = useRef(false);
	const openFragmentRef = useRef<number | null>(null);
	/* Tapes already streamed for the listed epoch, by fragment id. */
	const tapesRef = useRef(new Map<number, ForwardTapePoint[]>());

	const loadFragment = useCallback((frag: TrainedFragmentResponse) => {
		if (!frag) return;
		pinnedFragmentRef.current = true;
		setCurrentEpisode(null);
		currentEpisodeRef.current = null;
		setCurrentSymbol(frag.symbol);
		const mappedPoints =
			tapesRef.current.get(frag.id) ??
			(frag.points ?? []).map((pt, i) => ({
				x: i,
				y: pt.y,
				seq: pt.seq,
				time: toTimestampMs(pt.time),
			}));
		setPoints(mappedPoints);
		// The list carries no points: a fragment's stored trades are read from
		// the tape when it is opened.
		openFragmentRef.current = frag.id ?? null;
		if (mappedPoints.length === 0) {
			// The excursion's trades A..D stream over the hub socket (the
			// tape effect below draws them as they arrive).
			try {
				sendCommand(
					beginDetectionTape(
						frag.epoch,
						frag.symbol,
						frag.class ?? frag.direction,
						frag.c_tick,
					),
				);
			} catch (cause) {
				const message = cause instanceof Error ? cause.message : String(cause);
				console.error("historical runs: tape request not sent:", message);
				abortDetections(message);
				setDetectionsError(
					`tape of #${frag.id} ${frag.symbol} not requested: ${message}`,
				);
			}
		}
		setExcursionEvent({
			type: fragmentClass(frag.class, frag.direction),
			magnitude: frag.magnitude,
			marks: { A: frag.mark_a, B: frag.mark_b, C: frag.mark_c },
			entryIdx: frag.entry_idx,
			exitIdx: frag.exit_idx,
			predictedEntryIdx: frag.predicted_entry_idx ?? -1,
			predictedExitIdx: frag.predicted_exit_idx ?? -1,
			classCode: fragmentClassCode(frag.class, frag.direction),
		});
		setRawPrecursorTokens(frag.tokens ?? []);
		setFrozenAction(fragmentFrozenAction(frag.class, frag.direction));
		setDelayedLabel(fragmentDelayedLabel(frag.class, frag.direction));
	}, []);

	const selectFragment = useCallback(
		(idx: number) => {
			if (idx < 0 || idx >= trainedFragments.length) return;
			setSelectedFragmentIndex(idx);
			loadFragment(trainedFragments[idx]);
		},
		[trainedFragments, loadFragment],
	);

	const resumeLiveTape = useCallback(() => {
		if (historicalOnly) return;
		pinnedFragmentRef.current = false;
		setSelectedFragmentIndex(-1);
		setCurrentEpisode(null);
		currentEpisodeRef.current = null;
		setPoints([]);
		setExcursionEvent(null);
		setRawPrecursorTokens([]);
		setFrozenAction("ABSTAIN");
		setDelayedLabel("RESOLVING");
	}, [historicalOnly]);

	const replayFragment = useCallback(() => {
		if (
			selectedFragmentIndex < 0 ||
			selectedFragmentIndex >= trainedFragments.length
		)
			return;
		const frag = trainedFragments[selectedFragmentIndex];
		const rawPoints =
			frag.points && frag.points.length > 0 ? frag.points : points;
		const episodePoints = rawPoints.map((pt, i) => ({
			x: i,
			y: pt.y,
			seq: pt.seq,
			time: pt.time,
		}));
		if (episodePoints.length === 0) return;
		setCurrentEpisode(episodePoints);
		setPlaybackTick(0);
		setPlaybackPhase("PLAYING");
		setIsPlaying(true);
	}, [selectedFragmentIndex, trainedFragments, points]);

	/*
	 * Detections stream over the hub socket: the epochs detect labelled
	 * (newest first), then the chosen epoch's excursions. A request that
	 * cannot be sent, or a stream that fails, is shown, never swallowed.
	 */
	const online = useSelector(atoms["online"], (state) => state === "ONLINE");
	const detectionRuns = useSelector(
		detectionsStore,
		(state) => state.runs?.items ?? NO_RUNS,
	);
	const runsSlot = useSelector(detectionsStore, (state) => state.runs);
	const excursionsSlot = useSelector(
		detectionsStore,
		(state) => state.excursions,
	);
	const tapeSlot = useSelector(detectionsStore, (state) => state.tape);

	const request = useCallback(
		(command: Record<string, unknown>, what: string) => {
			try {
				sendCommand(command);
				setDetectionsError(null);
			} catch (cause) {
				const message = cause instanceof Error ? cause.message : String(cause);
				console.error(`historical runs: ${what} not requested:`, message);
				abortDetections(message);
				setDetectionsError(`${what} not requested: ${message}`);
			}
		},
		[],
	);

	// The runs are requested once the socket is up, and again after it
	// comes back if their stream failed (the hub ends a stream with its
	// client); a stream the hub failed is shown, not retried in a loop.
	useEffect(() => {
		if (!online) return;
		const runs = detectionsStore.state.runs;
		if (runs && runs.status !== "error") return;
		request(beginDetectionRuns(), "detection runs");
	}, [online, request]);

	useEffect(() => {
		if (detectionRun !== null || detectionRuns.length === 0) return;
		setDetectionRun(detectionRuns[0].epoch);
	}, [detectionRun, detectionRuns]);

	// A chosen epoch's excursions; a failed stream is retried on a new
	// choice or a reconnect, not in a loop.
	useEffect(() => {
		if (detectionRun === null || !online) return;
		const listed = detectionsStore.state.excursions;
		if (listed?.run === detectionRun && listed.status !== "error") return;
		setSelectedFragmentIndex(-1);
		pinnedFragmentRef.current = false;
		request(
			beginDetections(detectionRun),
			`detections of epoch ${detectionRun}`,
		);
	}, [detectionRun, online, request]);

	useEffect(() => {
		if (!excursionsSlot || excursionsSlot.run !== detectionRun) return;
		tapesRef.current.clear();
		setTrainedFragments(
			excursionsSlot.items.map((detection, idx) =>
				fragmentOf(excursionsSlot.run, detection, idx),
			),
		);
	}, [excursionsSlot, detectionRun]);

	// The open fragment's tape, drawn as its trades arrive.
	useEffect(() => {
		if (!tapeSlot || !pinnedFragmentRef.current) return;
		const open = trainedFragments.find((f) => f.id === openFragmentRef.current);
		if (
			!open ||
			open.epoch !== tapeSlot.run ||
			open.symbol !== tapeSlot.symbol ||
			open.c_tick !== tapeSlot.cTick
		)
			return;
		const mapped = tapeSlot.items.map((trade, i) => ({
			x: i,
			y: trade.price,
			seq: trade.seq,
			time: toTimestampMs(trade.at),
		}));
		setPoints(mapped);
		if (tapeSlot.status === "done") tapesRef.current.set(open.id, mapped);
	}, [tapeSlot, trainedFragments]);

	const streamError =
		detectionsError ??
		(runsSlot?.status === "error"
			? `detection runs: ${runsSlot.error}`
			: null) ??
		(excursionsSlot?.status === "error"
			? `detections of epoch ${excursionsSlot.run}: ${excursionsSlot.error}`
			: null) ??
		(tapeSlot?.status === "error"
			? `tape of ${tapeSlot.symbol} at tick ${tapeSlot.cTick}: ${tapeSlot.error}`
			: null);

	useEffect(() => {
		// Only the Historical runs tab auto-selects. Model training stays on the
		// live stream until the operator explicitly picks a fragment (or uses Live).
		if (!historicalOnly) return;
		if (trainedFragments.length === 0) return;
		if (selectedFragmentIndex >= 0) return;
		const lastIdx = trainedFragments.length - 1;
		setSelectedFragmentIndex(lastIdx);
		loadFragment(trainedFragments[lastIdx]);
	}, [trainedFragments, selectedFragmentIndex, loadFragment, historicalOnly]);

	useEffect(() => {
		const unsubPos = atoms["positionCount"].subscribe((count) => {
			setOpenPositionsCount(count);
		});
		setOpenPositionsCount(atoms["positionCount"].get());

		return () => {
			unsubPos?.unsubscribe?.();
		};
	}, []);

	// Resize observers
	useEffect(() => {
		const tapeTarget = tapeRef.current;
		if (!tapeTarget || typeof ResizeObserver === "undefined") return;

		const ro = new ResizeObserver((entries) => {
			if (entries[0]) {
				const { width, height } = entries[0].contentRect;
				if (width > 0 && height > 0) {
					setTapeDim({ width, height });
				}
			}
		});
		ro.observe(tapeTarget);
		return () => ro.disconnect();
	}, []);

	// A newly completed episode must wake playback even when the queue was empty.
	useEffect(() => {
		if (!isPlaying || currentEpisode || episodeQueue.length === 0) return;
		setCurrentEpisode(episodeQueue[0]);
		setPlaybackTick(0);
		setEpisodeQueue((queue) => queue.slice(1));
	}, [isPlaying, currentEpisode, episodeQueue]);

	// Playback Animation Loop - animates tape using actual tape time
	useEffect(() => {
		if (!isPlaying) return;

		let timer: number;
		if (playbackPhase === "PLAYING") {
			if (!currentEpisode || currentEpisode.length === 0) {
				return;
			}

			const validTimes = currentEpisode
				.map((p) => p.time)
				.filter((t): t is number => typeof t === "number" && t > 0);

			const hasValidSpan = validTimes.length >= 2;
			const tStart = hasValidSpan ? validTimes[0] : 0;
			const tEnd = hasValidSpan ? validTimes[validTimes.length - 1] : 0;
			const tapeSpanMs = hasValidSpan ? Math.max(0, tEnd - tStart) : 0;

			if (hasValidSpan && tapeSpanMs > 0) {
				const startTime = Date.now();
				const isManualReplay = pinnedFragmentRef.current || historicalOnly;
				// Playback at 1:1 real tape time. Cap extreme recordings at 30s.
				const maxDuration = isManualReplay
					? 30000
					: episodeQueue.length > 2
						? 1500
						: 5000;
				const targetDurationMs = Math.min(tapeSpanMs, maxDuration);
				const speedRate = tapeSpanMs / targetDurationMs;

				timer = window.setInterval(() => {
					const elapsedMs = Date.now() - startTime;
					if (elapsedMs >= targetDurationMs) {
						setPlaybackTick(currentEpisode.length - 1);
						setPlaybackPhase("EVALUATING");
						return;
					}

					const currentTapeTime = tStart + elapsedMs * speedRate;
					let targetTick = 0;
					for (let i = 0; i < currentEpisode.length; i++) {
						const ptTime = currentEpisode[i]?.time;
						if (typeof ptTime === "number" && ptTime <= currentTapeTime) {
							targetTick = i;
						} else if (typeof ptTime === "number" && ptTime > currentTapeTime) {
							break;
						}
					}
					setPlaybackTick(targetTick);
				}, 16);
			} else {
				// Fallback when no valid timestamps: step-based animation
				timer = window.setInterval(() => {
					setPlaybackTick((t) => {
						if (currentEpisode && t >= currentEpisode.length - 1) {
							setPlaybackPhase("EVALUATING");
							return t;
						}
						// Play fast: jump by 2 or 3 ticks depending on length
						const step = currentEpisode.length > 200 ? 3 : 2;
						return Math.min(t + step, currentEpisode.length - 1);
					});
				}, 16);
			}
		} else if (playbackPhase === "EVALUATING") {
			timer = window.setTimeout(() => {
				setCurrentEpisode(null);
				setPlaybackPhase("PLAYING");
				if (pinnedFragmentRef.current || historicalOnly) {
					setIsPlaying(false);
				}
			}, 1200); // Wait 1.2s at the end of the excursion to show the result
		}

		return () => {
			clearInterval(timer);
			clearTimeout(timer);
		};
	}, [
		isPlaying,
		playbackPhase,
		currentEpisode,
		historicalOnly,
		episodeQueue.length,
	]);

	// Update visible points from the animating episode
	useEffect(() => {
		currentEpisodeRef.current = currentEpisode;
		if (currentEpisode && playbackTick < currentEpisode.length) {
			setPoints(currentEpisode.slice(0, playbackTick + 1));
		}
	}, [currentEpisode, playbackTick]);

	// Subscribe to real live training measurements stream filtered by target symbol.
	// Historical-runs mode still reads stage/metrics for the side panels, but never
	// mutates the tape chart — selection must stick against BTC/USD hub updates.
	useEffect(() => {
		pendingEpisodeRef.current = [];
		currentEpisodeRef.current = null;
		setEpisodeQueue([]);
		if (!historicalOnly && !pinnedFragmentRef.current) {
			setPoints([]);
			setExcursionEvent(null);
		}
		lastSeenExcursionStartRef.current = -1;

		const handleFrame = (measurement: Value) => {
			{
				const activeSym = String(measurement.scope || "TRAINING");
				const tapePinned = historicalOnly || pinnedFragmentRef.current;

				if (
					!tapePinned &&
					lastSeenSymbolRef.current !== "" &&
					activeSym !== lastSeenSymbolRef.current
				) {
					// Symbol switched: purge any pending fragment so prices from different coins never mix
					pendingEpisodeRef.current = [];
					currentEpisodeRef.current = null;
					setEpisodeQueue([]);
					setPoints([]);
					setExcursionEvent(null);
					lastSeenExcursionStartRef.current = -1;
				}
				lastSeenSymbolRef.current = activeSym;
				if (!tapePinned) {
					setCurrentSymbol(activeSym);
				}

				const metricMap: Record<string, number> = measurement.metrics();
				{
				}

				const evalCount = metricMap.evaluated ?? 0;
				setEvaluatedCount(Math.floor(evalCount));

				// Update stage
				const sCode = metricMap.stage_code ?? 0;
				setStageCode(sCode);

				const provenance = measurement.provenance();
				if (provenance.stage_blocker !== undefined) {
					setStageBlocker(String(provenance.stage_blocker));
				}

				// Precursor & prediction
				const pLen = Math.floor(metricMap.precursor_length ?? 0);
				setPrecursorLength(pLen);

				let tokensList: string[] = [];
				let excDirection = "";
				if (provenance.precursor_tokens) {
					tokensList = String(provenance.precursor_tokens)
						.split(",")
						.filter(Boolean);
				}
				if (provenance.excursion_direction)
					excDirection = String(provenance.excursion_direction);
				let excStart: number | undefined;
				let excIgnition: number | undefined;
				let excExit: number | undefined;

				if (provenance.excursion_start !== undefined) {
					excStart = Number(provenance.excursion_start);
				}
				if (provenance.excursion_ignition !== undefined) {
					excIgnition = Number(provenance.excursion_ignition);
				}
				if (provenance.excursion_exit !== undefined) {
					excExit = Number(provenance.excursion_exit);
				}
				if (!tapePinned) {
					setRawPrecursorTokens(tokensList);
				}

				const markA =
					excStart !== undefined
						? Math.floor(excStart)
						: Math.floor(metricMap.mark_a ?? 0);
				const markB =
					excIgnition !== undefined
						? Math.floor(excIgnition)
						: Math.floor(metricMap.mark_b ?? 0);
				const markC =
					excExit !== undefined
						? Math.floor(excExit)
						: Math.floor(metricMap.mark_c ?? 0);

				if (
					excStart !== undefined &&
					excStart !== lastSeenExcursionStartRef.current
				) {
					// Finish the pending episode
					if (pendingEpisodeRef.current.length > 0) {
						const readyEpisode = [...pendingEpisodeRef.current];
						setEpisodeQueue((q) => [...q, readyEpisode].slice(-50)); // keep max 50 pending
					}
					pendingEpisodeRef.current = [];
					lastSeenExcursionStartRef.current = excStart;
				}

				const filledRaw = metricMap.paper_filled ?? 0;
				setIsPaperFilled(filledRaw === 1);

				if (!tapePinned) {
					const actRaw = metricMap.action;
					const frozenRaw =
						metricMap.frozen_prediction !== undefined
							? metricMap.frozen_prediction
							: actRaw;
					setFrozenAction(prediction(frozenRaw));
					setDelayedLabel(
						outcomeText(
							excDirection,
							metricMap.excursion_type ?? 0,
							metricMap.delayed_target,
						),
					);
				}

				// Historical held-out
				setHistOpportunities(Math.floor(metricMap.hist_opportunities ?? 0));
				setHistCorrectEnter(Math.floor(metricMap.hist_correct_enter ?? 0));
				setHistMissedEnter(Math.floor(metricMap.hist_missed_enter ?? 0));
				setHistFalseEnter(Math.floor(metricMap.hist_false_enter ?? 0));
				setHistMeanReturn(metricMap.hist_mean_return ?? 0);
				setHistLowerBound(metricMap.hist_lower_bound ?? 0);

				if (provenance.edge_samples) {
					const parsed = String(provenance.edge_samples)
						.split(",")
						.map((part) => Number(part.trim()))
						.filter((value) => Number.isFinite(value));
					if (parsed.length > 0) {
						setEdgeSamples(parsed);
					}
				}

				// Forward paper
				setFwdPredictions(Math.floor(metricMap.fwd_enter_predictions ?? 0));
				setFwdPaperTrades(Math.floor(metricMap.fwd_paper_trades ?? 0));
				setFwdPaperMeanReturn(metricMap.fwd_paper_mean_return ?? 0);
				setFwdPaperLowerBound(metricMap.fwd_paper_lower_bound ?? 0);

				// Extract price directly from measurement or metrics (which is log-return)
				let rawPrice: number | undefined;
				{
					const validPrices = [
						"price",
						"last_price",
						"mark_price",
						"best_price:bid",
						"best_price:ask",
						"trade_price",
						"mid",
						"close",
					];
					const found = validPrices
						.map((key) => metricMap[key])
						.find((value) => value !== undefined);
					if (found !== undefined) rawPrice = found;
				}

				if (rawPrice !== undefined) {
					const seqVal = Number(measurement.tick ?? 0n);
					const pointPrice = rawPrice;
					const ptTime = parseTimestamp(measurement.timestamp);
					if (!tapePinned) {
						pendingEpisodeRef.current.push({
							x: pendingEpisodeRef.current.length,
							y: pointPrice,
							seq: seqVal,
							time: ptTime,
						});
					}

					// Stream into visible points during active historical validation (stage 1)
					// when no episode is playing and no historical fragment is pinned.
					if (
						!tapePinned &&
						sCode === 1 &&
						currentEpisodeRef.current === null
					) {
						const pending = pendingEpisodeRef.current;
						setPoints(
							pending.map((pt, i) => ({
								x: i,
								y: pt.y,
								seq: pt.seq,
								time: pt.time,
							})),
						);
					}
				}

				const entryIdx =
					metricMap.agent_entry !== undefined && metricMap.agent_entry > 0
						? Math.floor(metricMap.agent_entry)
						: null;
				const exitIdx =
					metricMap.agent_exit !== undefined && metricMap.agent_exit > 0
						? Math.floor(metricMap.agent_exit)
						: null;
				const kind = excursionKind(excDirection, metricMap.excursion_type ?? 0);
				const hasMarks = markA > 0 || markB > 0 || markC > 0;

				if (
					!tapePinned &&
					!pinnedFragmentRef.current &&
					(kind || hasMarks || entryIdx !== null || exitIdx !== null)
				) {
					setExcursionEvent({
						type: kind,
						magnitude: metricMap.excursion_mag ?? 0,
						marks: { A: markA, B: markB, C: markC },
						entryIdx,
						exitIdx,
						predictedEntryIdx:
							metricMap.predicted_entry !== undefined &&
							metricMap.predicted_entry > 0
								? Math.floor(metricMap.predicted_entry)
								: null,
						predictedExitIdx:
							metricMap.predicted_exit !== undefined &&
							metricMap.predicted_exit > 0
								? Math.floor(metricMap.predicted_exit)
								: null,
					});
				}

				// String event tags are carried by the frame's provenance.
				if (provenance.excursion_event === "completed") {
					if (pendingEpisodeRef.current.length > 0) {
						const readyEpisode = [...pendingEpisodeRef.current];
						setEpisodeQueue((q) => [...q, readyEpisode].slice(-50));
					}
					pendingEpisodeRef.current = [];
				}
			}
		};

		const seen = new Set<string>();

		const updateFromState = (
			state: Record<string, Value> | undefined,
		) => {
			if (!state) return;
			const target = getTargetFrame(state, targetSymbol);
			if (target) {
				// One frame per symbol scope: feed it once per tick.
				const key = `${target.symbol}:${target.frame.tick}`;
				if (seen.has(key)) return;
				seen.add(key);
				handleFrame(target.frame);
			}
		};

		updateFromState(signals.training?.state);

		const unsub = signals.training.subscribe((state) => {
			updateFromState(state);
		});

		return () => {
			unsub?.unsubscribe?.();
		};
	}, [targetSymbol, historicalOnly]);

	// Scales for real tape rendering
	const {
		xScale,
		yScale,
		minPrice,
		maxPrice,
		midPrice,
		currentPoints,
		lineGenerator,
		areaGenerator,
		timeTicks,
		timeSpanLabel,
	} = useMemo(() => {
		const activeTape =
			currentEpisode && currentEpisode.length > 0 ? currentEpisode : points;
		const isExcursion =
			historicalOnly ||
			selectedFragmentIndex >= 0 ||
			pinnedFragmentRef.current ||
			currentEpisode !== null ||
			excursionEvent !== null;

		const totalFrames = activeTape.length;
		const maxIndex = isExcursion
			? Math.max(1, totalFrames - 1)
			: Math.max(totalFrames - 1, 50);

		// Pad horizontal domain so boundary markers (A/B/C) and labels never clip against SVG viewport edges
		const xPad =
			tapeDim.width > 0 ? Math.min(32, Math.max(18, tapeDim.width * 0.025)) : 0;
		const xs = d3
			.scaleLinear()
			.domain([0, maxIndex])
			.range([xPad, Math.max(xPad + 1, tapeDim.width - xPad)]);

		let minY = Number.POSITIVE_INFINITY;
		let maxY = Number.NEGATIVE_INFINITY;
		for (const pt of activeTape) {
			if (pt.y < minY) minY = pt.y;
			if (pt.y > maxY) maxY = pt.y;
		}

		if (!Number.isFinite(minY) || !Number.isFinite(maxY)) {
			minY = 0;
			maxY = 100;
		} else if (minY === maxY) {
			const delta = minY !== 0 ? Math.abs(minY) * 0.005 : 1;
			minY -= delta;
			maxY += delta;
		} else {
			const pad = (maxY - minY) * 0.15;
			minY -= pad;
			maxY += pad;
		}

		const ys = d3
			.scaleLinear()
			.domain([minY, maxY])
			.range([tapeDim.height - 35, 35]);

		const lg = d3
			.line<ForwardTapePoint>()
			.x((d) => xs(d.x))
			.y((d) => ys(d.y))
			.curve(d3.curveMonotoneX);

		const ag = d3
			.area<ForwardTapePoint>()
			.x((d) => xs(d.x))
			.y0(tapeDim.height - 20)
			.y1((d) => ys(d.y))
			.curve(d3.curveMonotoneX);

		// Calculate relative time ticks and span across evaluated tape fragment
		const validTimes = activeTape
			.map((p) => p.time)
			.filter((t): t is number => typeof t === "number" && t > 0);

		let timeSpanLabel = "";
		const timeTicks: Array<{
			x: number;
			label: string;
			align: "start" | "middle" | "end";
		}> = [];
		const availableWidth = Math.max(1, tapeDim.width - 2 * xPad);

		if (validTimes.length >= 2 && tapeDim.width > 0 && activeTape.length > 0) {
			const tStart = validTimes[0];
			const tEnd = validTimes[validTimes.length - 1];
			const spanMs = Math.max(0, tEnd - tStart);
			timeSpanLabel = formatDurationSpan(spanMs);

			const maxTicks = Math.min(6, Math.max(2, activeTape.length));
			const numTicks = Math.max(
				2,
				Math.min(maxTicks, Math.floor(availableWidth / 125)),
			);

			const rawIndices = Array.from({ length: numTicks }, (_, i) =>
				Math.min(
					activeTape.length - 1,
					Math.round((i / Math.max(1, numTicks - 1)) * (activeTape.length - 1)),
				),
			);
			const tickIndices = Array.from(new Set(rawIndices));

			const times = tickIndices.map((idx) => {
				const ptTime = activeTape[idx]?.time;
				if (ptTime && ptTime > 0) return ptTime;
				const fraction = idx / Math.max(1, activeTape.length - 1);
				return tStart + fraction * spanMs;
			});

			for (let i = 0; i < tickIndices.length; i++) {
				const ptIdx = tickIndices[i];
				const px = xs(activeTape[ptIdx].x);
				const relMs = Math.max(0, times[i] - tStart);

				let label: string;
				if (i === 0) {
					const clock = formatClockTime(tStart, true);
					label = clock !== "—" ? `t₀ (${clock})` : "t₀ (+0.00s)";
				} else if (spanMs < 10_000) {
					label = `+${(relMs / 1000).toFixed(2)}s`;
				} else if (spanMs < 60_000) {
					label = `+${(relMs / 1000).toFixed(1)}s`;
				} else {
					const totalSec = Math.floor(relMs / 1000);
					const mins = Math.floor(totalSec / 60);
					const secs = totalSec % 60;
					label =
						mins > 0
							? `+${mins}m${secs > 0 ? ` ${secs}s` : ""}`
							: `+${totalSec}s`;
				}

				timeTicks.push({
					x: px,
					label,
					align:
						i === 0 ? "start" : i === tickIndices.length - 1 ? "end" : "middle",
				});
			}
		} else if (activeTape.length >= 2 && tapeDim.width > 0) {
			const seqs = activeTape
				.map((p) => p.seq)
				.filter((s): s is number => typeof s === "number" && s > 0);

			const maxTicks = Math.min(5, Math.max(2, activeTape.length));
			const numTicks = Math.max(
				2,
				Math.min(maxTicks, Math.floor(availableWidth / 130)),
			);

			const rawIndices = Array.from({ length: numTicks }, (_, i) =>
				Math.min(
					activeTape.length - 1,
					Math.round((i / Math.max(1, numTicks - 1)) * (activeTape.length - 1)),
				),
			);
			const tickIndices = Array.from(new Set(rawIndices));
			const baseSeq = seqs.length > 0 ? seqs[0] : null;

			for (let i = 0; i < tickIndices.length; i++) {
				const ptIdx = tickIndices[i];
				const px = xs(activeTape[ptIdx].x);
				const curSeq = activeTape[ptIdx]?.seq;
				let label: string;
				if (i === 0) {
					label = curSeq !== undefined ? `t₀ (#${curSeq})` : "t₀ (+0)";
				} else if (baseSeq !== null && curSeq !== undefined) {
					const diff = curSeq - baseSeq;
					label = `+${diff} (#${curSeq})`;
				} else {
					label = `+${ptIdx}`;
				}
				timeTicks.push({
					x: px,
					label,
					align:
						i === 0 ? "start" : i === tickIndices.length - 1 ? "end" : "middle",
				});
			}
		} else if (activeTape.length === 1 && tapeDim.width > 0) {
			const t = activeTape[0].time;
			const clock = t && t > 0 ? formatClockTime(t, true) : "";
			timeTicks.push({
				x: xs(0),
				label: clock && clock !== "—" ? `t₀ (${clock})` : "t₀",
				align: "start",
			});
		}

		return {
			xScale: xs,
			yScale: ys,
			minPrice: minY,
			maxPrice: maxY,
			midPrice: (minY + maxY) / 2,
			currentPoints: points,
			lineGenerator: lg,
			areaGenerator: ag,
			timeTicks,
			timeSpanLabel,
		};
	}, [
		points,
		currentEpisode,
		tapeDim.width,
		tapeDim.height,
		historicalOnly,
		selectedFragmentIndex,
		excursionEvent,
	]);

	const isForward = stageCode >= 2;
	const precursorTokens = useMemo(() => {
		if (rawPrecursorTokens.length > 0) {
			return rawPrecursorTokens.map((t) => `[${t}]`);
		}
		if (precursorLength === 0) {
			return ["―"];
		}
		return ["unavailable"];
	}, [rawPrecursorTokens, precursorLength]);

	return (
		<Flex.Column className="h-full min-h-0 overflow-hidden bg-(--bg)">
			<Flex className="h-3/5 min-h-0">
				{historicalOnly && (
					<Section
						fit="pane"
						className="w-56 shrink-0 border-(--line) border-r"
						data-l="historical-runs-list"
					>
						<Section.Header
							size="s"
							title="Historical runs"
							meta={trainedFragments.length}
						/>
						{detectionRuns.length > 0 && (
							<select
								className="shrink-0 border-(--line) border-b bg-(--sunken) px-2 py-1 text-[10px] text-(--f2)"
								value={detectionRun ?? ""}
								onChange={(event) => setDetectionRun(event.target.value)}
								aria-label="Select detected tick epoch"
								data-l="historical-runs-epoch"
							>
								{detectionRuns.map((entry) => (
									<option key={entry.epoch} value={entry.epoch}>
										{new Date(Number(BigInt(entry.epoch) / 1_000_000n))
											.toISOString()
											.slice(0, 16)}{" "}
										· {entry.fields.ticks} ticks · {entry.fields.up} up /{" "}
										{entry.fields.down} down
									</option>
								))}
							</select>
						)}
						{streamError && (
							<Alert size="s" data-l="historical-runs-error">
								{streamError}
							</Alert>
						)}
						<Section.Body>
							{trainedFragments.length === 0 ? (
								<Typography.Mono size="s" tone="f4">
									{runsSlot?.status === "done" && detectionRuns.length === 0
										? "No epoch is labelled yet: run detect on a collected tick epoch."
										: excursionsSlot?.status === "done"
											? `Epoch ${excursionsSlot.run} stores no detections.`
											: "Streaming detections from the hub…"}
								</Typography.Mono>
							) : (
								trainedFragments
									.map((frag, idx) => ({ frag, idx }))
									.reverse()
									.map(({ frag, idx }) => {
										const active = idx === selectedFragmentIndex;
										const cls = fragmentClassCode(frag.class, frag.direction);
										return (
											<ListRun
												key={frag.id ?? idx}
												active={active}
												onClick={() => selectFragment(idx)}
												label={`#${frag.id} ${frag.symbol}`}
												mark={cls}
												detail={(frag.direction || "").toUpperCase()}
												value={`${(frag.magnitude * 100).toFixed(2)}%`}
											/>
										);
									})
							)}
						</Section.Body>
					</Section>
				)}
				<Flex.Column className="min-h-0 min-w-0 flex-1 border-(--line) border-r bg-(--sunken)">
					<Section.Header
						size="s"
						meta={
							currentPoints.length > 0 ? (
								<Badge
									variant="info"
									label={`Δ${currentPoints[currentPoints.length - 1].y.toLocaleString(
										undefined,
										{
											minimumFractionDigits: 5,
											maximumFractionDigits: 5,
										},
									)}`}
								/>
							) : undefined
						}
					>
						<Flex.Row align="center" gap={2} className="min-w-0">
							<Badge data-l="tape-title" label="HISTORICAL RUNS" />
							<Badge variant="disabled" label={currentSymbol} />

							{trainedFragments.length > 0 && (
								<Flex.Row
									align="center"
									gap={1}
									data-l="fragment-selector"
								>
									<Button
										size="xs"
										variant="outline"
										shape="icon"
										onClick={() => selectFragment(selectedFragmentIndex - 1)}
										disabled={selectedFragmentIndex <= 0}
										title="Previous fragment"
									>
										◀
									</Button>
									<select
										value={selectedFragmentIndex}
										onChange={(e) => selectFragment(Number(e.target.value))}
										className="max-w-70 cursor-pointer border border-(--line) bg-(--surface) px-1.5 py-0.5 text-[10px] text-(--f1)"
										aria-label="Select historical training run"
									>
										{!historicalOnly && selectedFragmentIndex < 0 && (
											<option value={-1}>Live stream…</option>
										)}
										{trainedFragments.map((frag, idx) => (
											<option key={frag.id ?? idx} value={idx}>
												#{frag.id} {frag.symbol} [
												{fragmentClassCode(frag.class, frag.direction)}] (
												{frag.direction.toUpperCase()}{" "}
												{(frag.magnitude * 100).toFixed(2)}%)
											</option>
										))}
									</select>
									<Button
										size="xs"
										variant="outline"
										shape="icon"
										onClick={() => selectFragment(selectedFragmentIndex + 1)}
										disabled={
											selectedFragmentIndex >= trainedFragments.length - 1
										}
										title="Next fragment"
									>
										▶
									</Button>
									<Button
										size="xs"
										variant="solid"
										tone="accent"
										onClick={replayFragment}
										disabled={selectedFragmentIndex < 0}
										title="Replay this fragment animation"
									>
										Replay
									</Button>
									{!historicalOnly && selectedFragmentIndex >= 0 && (
										<Button
											size="xs"
											variant="outline"
											tone="accent"
											onClick={resumeLiveTape}
											title="Return to live training tape"
											data-l="resume-live-tape"
										>
											Live
										</Button>
									)}
								</Flex.Row>
							)}

							<Typography.Mono size="s" tone="f1">
								{points.length} frames evaluated
							</Typography.Mono>
							{timeSpanLabel && (
								<Badge
									variant="info"
									label={timeSpanLabel}
									title="Time-scale span of this tape fragment"
								/>
							)}
						</Flex.Row>
					</Section.Header>

					{/* Confirmed Excursion Banner */}
					<AnimatePresence>
						{excursionEvent?.type && (
							<motion.div
								initial={{ height: 0, opacity: 0 }}
								animate={{ height: 22, opacity: 1 }}
								exit={{ height: 0, opacity: 0 }}
								className={cn(
									"border-b flex items-center px-3 text-[10px] font-bold z-10 shrink-0",
									excursionEvent.type === "UPWARD EXCURSION"
										? "bg-(--up)/10 border-(--up)/20 text-(--up)"
										: excursionEvent.type === "UP FRICTION"
											? "bg-(--warn)/10 border-(--warn)/20 text-(--warn)"
											: excursionEvent.type === "DOWNWARD EXCURSION"
												? "bg-(--down)/10 border-(--down)/20 text-(--down)"
												: "bg-(--sunken) border-(--line) text-(--f3)",
								)}
							>
								<span>
									{excursionEvent.type}{" "}
									{excursionEvent.classCode ? (
										<span
											data-l="fragment-class"
											className="ml-1 font-mono text-[9px] uppercase tracking-wider opacity-90"
										>
											[{excursionEvent.classCode}]
										</span>
									) : null}{" "}
									<span className="text-(--f3) ml-1 font-normal">
										confirmed
									</span>
								</span>
								<span className="ml-auto">
									{signedPercent(excursionEvent.magnitude)}
								</span>
							</motion.div>
						)}
					</AnimatePresence>

					{/* SVG Tape Canvas */}
					<div
						ref={tapeRef}
						className="flex-1 relative overflow-hidden bg-(--sunken)"
					>
						<Scanlines variant="plate" className="pointer-events-none z-10" />

						{points.length === 0 && (
							<div className="absolute inset-0 flex items-center justify-center text-(--f4) text-xs tracking-wider z-5">
								{historicalOnly
									? trainedFragments.length > 0
										? "Select a historical training run"
										: "No trained fragments yet — waiting for rehearsal to publish"
									: stageCode === 1 && selectedFragmentIndex < 0
										? `Awaiting training stream for ${currentSymbol}...`
										: trainedFragments.length > 0
											? "Select a trained fragment above to inspect"
											: `Awaiting historical replay tape stream for ${currentSymbol}...`}
							</div>
						)}

						{tapeDim.width > 0 && points.length > 0 && (
							<svg
								width={tapeDim.width}
								height={tapeDim.height}
								className="absolute inset-0 z-5"
								aria-label={`Tape of ${currentSymbol}`}
							>
								<title>{currentSymbol}</title>
								<defs>
									<linearGradient
										id="tapeScreenAreaGradient"
										x1="0"
										y1="0"
										x2="0"
										y2="1"
									>
										<stop
											offset="0%"
											stopColor="var(--acc)"
											stopOpacity="0.14"
										/>
										<stop
											offset="100%"
											stopColor="var(--acc)"
											stopOpacity="0.0"
										/>
									</linearGradient>
								</defs>
								{/* Y-Axis Price Scale Ticks and Grid */}
								<g className="text-[9px] font-mono text-(--f4) select-none pointer-events-none">
									<line
										x1="0"
										y1={yScale(maxPrice)}
										x2={tapeDim.width}
										y2={yScale(maxPrice)}
										stroke="var(--line)"
										strokeDasharray="2 4"
										opacity="0.35"
									/>
									<text x={8} y={yScale(maxPrice) - 4} fill="var(--f3)">
										{maxPrice.toLocaleString(undefined, {
											minimumFractionDigits: 5,
											maximumFractionDigits: 5,
										})}
									</text>

									<line
										x1="0"
										y1={yScale(midPrice)}
										x2={tapeDim.width}
										y2={yScale(midPrice)}
										stroke="var(--line)"
										strokeDasharray="2 4"
										opacity="0.35"
									/>
									<text x={8} y={yScale(midPrice) - 4} fill="var(--f4)">
										{midPrice.toLocaleString(undefined, {
											minimumFractionDigits: 5,
											maximumFractionDigits: 5,
										})}
									</text>

									<line
										x1="0"
										y1={yScale(minPrice)}
										x2={tapeDim.width}
										y2={yScale(minPrice)}
										stroke="var(--line)"
										strokeDasharray="2 4"
										opacity="0.35"
									/>
									<text x={8} y={yScale(minPrice) - 4} fill="var(--f4)">
										{minPrice.toLocaleString(undefined, {
											minimumFractionDigits: 5,
											maximumFractionDigits: 5,
										})}
									</text>
								</g>

								{/* Phosphor Area Fill */}
								{currentPoints.length > 1 && (
									<path
										d={areaGenerator(currentPoints) || undefined}
										fill="url(#tapeScreenAreaGradient)"
									/>
								)}

								{/* Price Trajectory Path */}
								<path
									d={lineGenerator(currentPoints) || undefined}
									fill="none"
									stroke="var(--acc)"
									strokeWidth="1.8"
								/>

								{/* Leading Point & Real Price Badge */}
								{currentPoints.length > 0 && (
									<g
										transform={`translate(${xScale(currentPoints[currentPoints.length - 1].x)}, ${yScale(currentPoints[currentPoints.length - 1].y)})`}
									>
										<circle
											r={7}
											fill="none"
											stroke="var(--acc)"
											strokeWidth="0.8"
											opacity="0.4"
										/>
										<circle r={3.5} fill="var(--acc)" />
										<text
											x={-8}
											y={-8}
											fill="var(--acc)"
											fontSize="9px"
											fontWeight="bold"
											textAnchor="end"
										>
											Δ
											{currentPoints[currentPoints.length - 1].y.toLocaleString(
												undefined,
												{
													minimumFractionDigits: 5,
													maximumFractionDigits: 5,
												},
											)}
										</text>
									</g>
								)}

								{/* Hindsight Markers A, B, C and Decisions */}
								{excursionEvent &&
									points.length > 0 &&
									(isForward ||
										playbackPhase === "EVALUATING" ||
										excursionEvent.marks.A >= 0 ||
										excursionEvent.marks.B >= 0 ||
										excursionEvent.marks.C >= 0) &&
									(() => {
										const resolveIdx = (val: number | null): number | null => {
											if (
												val === null ||
												val === undefined ||
												val < 0 ||
												points.length === 0
											)
												return null;
											// 1. Exact sequence index match
											for (let i = 0; i < points.length; i++) {
												const seq = points[i].seq;
												if (seq !== undefined && seq >= 0 && seq === val)
													return i;
											}
											// 2. Window-local relative index (entry_idx, exit_idx)
											if (val >= 0 && val < points.length) {
												return val;
											}
											// 3. Nearest sequence index match within or near sequence range
											if (points.length > 0) {
												let nearestIdx = 0;
												let nearestDist = Math.abs((points[0].seq ?? 0) - val);
												for (let i = 1; i < points.length; i++) {
													const seq = points[i].seq ?? 0;
													const dist = Math.abs(seq - val);
													if (dist < nearestDist) {
														nearestDist = dist;
														nearestIdx = i;
													}
												}
												if (nearestDist <= 50) {
													return nearestIdx;
												}
											}
											return null;
										};

										const markAIdx =
											excursionEvent.marks.A >= 0
												? resolveIdx(excursionEvent.marks.A)
												: null;
										const markBIdx =
											excursionEvent.marks.B >= 0
												? resolveIdx(excursionEvent.marks.B)
												: null;
										const markCIdx =
											excursionEvent.marks.C >= 0
												? resolveIdx(excursionEvent.marks.C)
												: null;

										const marksList: { name: string; idx: number }[] = [];
										if (markAIdx !== null)
											marksList.push({ name: "A", idx: markAIdx });
										if (markBIdx !== null)
											marksList.push({ name: "B", idx: markBIdx });
										if (markCIdx !== null)
											marksList.push({ name: "C", idx: markCIdx });

										const entryPtIdx =
											excursionEvent.entryIdx !== null &&
											excursionEvent.entryIdx >= 0
												? resolveIdx(excursionEvent.entryIdx)
												: null;
										const exitPtIdx =
											excursionEvent.exitIdx !== null &&
											excursionEvent.exitIdx >= 0
												? resolveIdx(excursionEvent.exitIdx)
												: null;
										const predictedEntryPtIdx =
											excursionEvent.predictedEntryIdx !== null &&
											excursionEvent.predictedEntryIdx >= 0
												? resolveIdx(excursionEvent.predictedEntryIdx)
												: null;
										const predictedExitPtIdx =
											excursionEvent.predictedExitIdx !== null &&
											excursionEvent.predictedExitIdx >= 0
												? resolveIdx(excursionEvent.predictedExitIdx)
												: null;

										return (
											<g>
												{/* Shaded Precursor Horizon (A → B) */}
												{markAIdx !== null &&
													markBIdx !== null &&
													markBIdx > markAIdx &&
													markAIdx < points.length &&
													markBIdx <= points.length && (
														<g className="precursor-horizon">
															<rect
																x={xScale(markAIdx)}
																y={0}
																width={Math.max(
																	0,
																	xScale(markBIdx) - xScale(markAIdx),
																)}
																height={tapeDim.height - 20}
																fill="color-mix(in srgb, var(--acc) 6%, transparent)"
																stroke="var(--acc)"
																strokeWidth={1}
																strokeDasharray="2 4"
																strokeOpacity={0.35}
															/>
															<text
																x={xScale(markAIdx) + 6}
																y={16}
																fill="var(--acc)"
																fontSize="8px"
																fontWeight="bold"
																letterSpacing="0.08em"
																className="select-none pointer-events-none font-mono opacity-80 uppercase"
															>
																{xScale(markBIdx) - xScale(markAIdx) > 90
																	? "Precursor Phase (A→B)"
																	: xScale(markBIdx) - xScale(markAIdx) > 40
																		? "Precursor (A→B)"
																		: "(A→B)"}
															</text>
														</g>
													)}

												{/* Shaded Excursion Target Horizon (B → C) */}
												{markBIdx !== null &&
													markCIdx !== null &&
													markCIdx > markBIdx &&
													markBIdx < points.length &&
													markCIdx <= points.length && (
														<g className="excursion-horizon">
															<rect
																x={xScale(markBIdx)}
																y={0}
																width={Math.max(
																	0,
																	xScale(markCIdx) - xScale(markBIdx),
																)}
																height={tapeDim.height - 20}
																fill="color-mix(in srgb, var(--up) 7%, transparent)"
																stroke="var(--up)"
																strokeWidth={1}
																strokeDasharray="2 4"
																strokeOpacity={0.35}
															/>
															<text
																x={xScale(markBIdx) + 6}
																y={16}
																fill="var(--up)"
																fontSize="8px"
																fontWeight="bold"
																letterSpacing="0.08em"
																className="select-none pointer-events-none font-mono opacity-80 uppercase"
															>
																{xScale(markCIdx) - xScale(markBIdx) > 90
																	? "Excursion Target (B→C)"
																	: xScale(markCIdx) - xScale(markBIdx) > 40
																		? "Target (B→C)"
																		: "(B→C)"}
															</text>
														</g>
													)}

												{marksList.map(({ name, idx }) => {
													if (idx < 0 || idx >= points.length) return null;
													const xPos = xScale(idx);
													return (
														<g key={name} transform={`translate(${xPos}, 0)`}>
															<line
																x1={0}
																y1={15}
																x2={0}
																y2={tapeDim.height}
																stroke="var(--info)"
																strokeWidth="1"
																strokeDasharray="2 4"
																opacity="0.5"
															/>
															<rect
																x={-7}
																y={8}
																width={14}
																height={14}
																fill="var(--sunken)"
																stroke="var(--info)"
																strokeWidth="1"
															/>
															<text
																x={0}
																y={18}
																fill="var(--info)"
																fontSize="9px"
																fontWeight="bold"
																textAnchor="middle"
															>
																{name}
															</text>
														</g>
													);
												})}

												{/* Ground-truth ENTER sweet spot */}
												{entryPtIdx !== null &&
													entryPtIdx >= 0 &&
													entryPtIdx < points.length && (
														<g
															transform={`translate(${xScale(entryPtIdx)}, ${yScale(points[entryPtIdx].y)})`}
														>
															<circle r={4} fill="var(--up)" />
															<text
																x={6}
																y={-6}
																fill="var(--up)"
																fontSize="9px"
																fontWeight="bold"
															>
																ENTER
															</text>
															<line
																y2={tapeDim.height}
																stroke="var(--up)"
																opacity="0.3"
															/>
														</g>
													)}

												{/* Post-teach predicted ENTER (outline, distinct from GT) */}
												{predictedEntryPtIdx !== null &&
													predictedEntryPtIdx >= 0 &&
													predictedEntryPtIdx < points.length && (
														<g
															transform={`translate(${xScale(predictedEntryPtIdx)}, ${yScale(points[predictedEntryPtIdx].y)})`}
														>
															<circle
																r={6}
																fill="none"
																stroke="var(--up)"
																strokeWidth={2}
																strokeDasharray="3 2"
															/>
															<text
																x={8}
																y={12}
																fill="var(--up)"
																fontSize="8px"
																fontWeight="bold"
															>
																PREDICTED ENTER
															</text>
														</g>
													)}

												{/* Paper Entry Fill Marker */}
												{isForward &&
													isPaperFilled &&
													entryPtIdx !== null &&
													entryPtIdx >= 0 &&
													entryPtIdx < points.length && (
														<g
															transform={`translate(${xScale(entryPtIdx)}, ${yScale(points[entryPtIdx].y) + 14})`}
														>
															<rect
																x={4}
																y={-10}
																width={90}
																height={12}
																fill="var(--sunken)"
																stroke="var(--up)"
																strokeWidth={0.5}
															/>
															<text
																x={6}
																y={-1}
																fill="var(--up)"
																fontSize="8px"
																fontWeight="bold"
															>
																PAPER ENTRY FILL
															</text>
														</g>
													)}

												{/* Ground-truth EXIT sweet spot */}
												{exitPtIdx !== null &&
													exitPtIdx >= 0 &&
													exitPtIdx < points.length && (
														<g
															transform={`translate(${xScale(exitPtIdx)}, ${yScale(points[exitPtIdx].y)})`}
														>
															<circle r={4} fill="var(--down)" />
															<text
																x={6}
																y={-6}
																fill="var(--down)"
																fontSize="9px"
																fontWeight="bold"
															>
																EXIT
															</text>
															<line
																y2={tapeDim.height}
																stroke="var(--down)"
																opacity="0.3"
															/>
														</g>
													)}

												{predictedExitPtIdx !== null &&
													predictedExitPtIdx >= 0 &&
													predictedExitPtIdx < points.length && (
														<g
															transform={`translate(${xScale(predictedExitPtIdx)}, ${yScale(points[predictedExitPtIdx].y)})`}
														>
															<circle
																r={6}
																fill="none"
																stroke="var(--down)"
																strokeWidth={2}
																strokeDasharray="3 2"
															/>
															<text
																x={8}
																y={12}
																fill="var(--down)"
																fontSize="8px"
																fontWeight="bold"
															>
																PREDICTED EXIT
															</text>
														</g>
													)}
											</g>
										);
									})()}

								{/* X-Axis Time Scale Ticks and Grid */}
								{timeTicks.length > 0 && (
									<g className="text-[9px] font-mono select-none pointer-events-none">
										<line
											x1="0"
											y1={tapeDim.height - 20}
											x2={tapeDim.width}
											y2={tapeDim.height - 20}
											stroke="var(--line)"
											opacity="0.6"
										/>
										{timeTicks.map((tick) => (
											<g key={`time-tick-${tick.x}-${tick.label}`}>
												<line
													x1={tick.x}
													y1={24}
													x2={tick.x}
													y2={tapeDim.height - 20}
													stroke="var(--line)"
													strokeDasharray="2 4"
													opacity="0.25"
												/>
												<line
													x1={tick.x}
													y1={tapeDim.height - 20}
													x2={tick.x}
													y2={tapeDim.height - 15}
													stroke="var(--f4)"
													opacity="0.8"
												/>
												<text
													x={tick.x}
													y={tapeDim.height - 5}
													fill="var(--f3)"
													fontSize="9px"
													textAnchor={tick.align}
													dx={
														tick.align === "start"
															? 4
															: tick.align === "end"
																? -4
																: 0
													}
												>
													{tick.label}
												</text>
											</g>
										))}
									</g>
								)}
							</svg>
						)}
					</div>

					{/* Temporal Precursor Fragment Bar */}
					<div className="h-7 border-t border-(--line) bg-(--sunken) flex items-center px-3 justify-between text-[10px] text-(--f3) shrink-0">
						<div
							className="flex items-center gap-1.5"
							data-l="temporal-precursor"
						>
							<span className="text-(--f4) uppercase tracking-wider font-bold">
								Temporal Precursor:
							</span>
							<span className="text-(--acc) font-mono">
								{precursorTokens.join(" → ")}
							</span>
						</div>
						<div className="flex items-center gap-2" data-l="abc-markers">
							<span className="text-(--f4)">Class:</span>
							<span
								data-l="fragment-class-detail"
								className="font-bold text-(--f1) uppercase"
							>
								{excursionEvent?.classCode ?? "—"}
							</span>
							<span className="text-(--f4)">Boundaries:</span>
							<span>A: {excursionEvent?.marks.A ?? 0}</span>
							<span>→</span>
							<span>
								B: {excursionEvent?.marks.B ?? 0}
								{excursionEvent &&
								excursionEvent.marks.B > excursionEvent.marks.A ? (
									<span className="text-(--acc) ml-1 font-mono text-[9px]">
										(+{excursionEvent.marks.B - excursionEvent.marks.A})
									</span>
								) : null}
							</span>
							<span>→</span>
							<span>
								C: {excursionEvent?.marks.C ?? 0}
								{excursionEvent &&
								excursionEvent.marks.C > excursionEvent.marks.B ? (
									<span className="text-(--up) ml-1 font-mono text-[9px]">
										(+{excursionEvent.marks.C - excursionEvent.marks.B})
									</span>
								) : null}
							</span>
						</div>
						<div className="flex items-center gap-2">
							<span className="text-(--f4)">Pre-Outcome Prediction:</span>
							<span
								data-l="frozen-prediction"
								className="font-bold text-(--acc)"
							>
								{frozenAction}
							</span>
							<span className="text-(--f4)">Actual Delayed Label:</span>
							<span data-l="delayed-label" className="font-bold text-(--f1)">
								{delayedLabel}
							</span>
						</div>
					</div>
				</Flex.Column>

				<Section fit="pane" className="w-72 shrink-0">
					<Section.Header
						size="s"
						title={
							isForward ? "Forward paper authority" : "Historical validation"
						}
						meta={`${evaluatedCount.toLocaleString()} frames`}
					/>
					<Section.Body scroll className="flex flex-col">
						<Section fit="content" className="border-(--line) border-b">
							<Section.Header
								size="s"
								title="Historical prediction outcomes"
								meta={`${histOpportunities} opps`}
							/>

							{/* Visual Stacked Outcome Distribution Bar for Number-Blind Inspection */}
							{(() => {
								const totalOutcomes =
									histCorrectEnter + histFalseEnter + histMissedEnter;
								const correctPct =
									totalOutcomes > 0
										? (histCorrectEnter / totalOutcomes) * 100
										: 0;
								const falsePct =
									totalOutcomes > 0
										? (histFalseEnter / totalOutcomes) * 100
										: 0;
								const missedPct =
									totalOutcomes > 0
										? (histMissedEnter / totalOutcomes) * 100
										: 0;
								return (
									<div className="flex flex-col gap-1">
										<div className="h-2 w-full rounded-full bg-(--sunken) border border-(--line) flex overflow-hidden">
											{correctPct > 0 && (
												<div
													style={{ width: `${correctPct}%` }}
													className="bg-(--up) h-full transition-all duration-300"
													title={`Correct: ${correctPct.toFixed(1)}%`}
												/>
											)}
											{falsePct > 0 && (
												<div
													style={{ width: `${falsePct}%` }}
													className="bg-(--down) h-full transition-all duration-300"
													title={`False: ${falsePct.toFixed(1)}%`}
												/>
											)}
											{missedPct > 0 && (
												<div
													style={{ width: `${missedPct}%` }}
													className="bg-(--warn) h-full transition-all duration-300"
													title={`Missed: ${missedPct.toFixed(1)}%`}
												/>
											)}
											{totalOutcomes === 0 && (
												<div className="w-full h-full bg-(--line)" />
											)}
										</div>
										<div className="flex justify-between text-[9px] text-(--f4)">
											<span className="flex items-center gap-1">
												<span className="inline-block w-1.5 h-1.5 rounded-full bg-(--up)" />
												Correct
											</span>
											<span className="flex items-center gap-1">
												<span className="inline-block w-1.5 h-1.5 rounded-full bg-(--down)" />
												False
											</span>
											<span className="flex items-center gap-1">
												<span className="inline-block w-1.5 h-1.5 rounded-full bg-(--warn)" />
												Missed
											</span>
										</div>
									</div>
								);
							})()}

							<div className="flex justify-between items-baseline text-[11px]">
								<span className="text-(--f4)">Mean Quote Return:</span>
								<span
									className="text-(--f1) font-bold"
									data-metric="hist_mean_return"
									data-format="insufficient_if_zero"
								>
									{histCorrectEnter + histFalseEnter > 0
										? basis(histMeanReturn)
										: "—"}
								</span>
							</div>
							<div className="flex justify-between items-baseline text-[10px]">
								<span className="text-(--f4)">Mean − Standard Error:</span>
								<span
									className="text-(--acc) font-bold"
									data-metric="hist_lower_bound"
									data-format="insufficient_if_zero"
								>
									{histCorrectEnter + histFalseEnter > 1
										? basis(histLowerBound)
										: "—"}
								</span>
							</div>

							{/* Statistical Lower Bound Visual Edge Meter */}
							<div className="flex flex-col gap-0.5 pt-0.5">
								<div className="flex justify-between text-[9px] text-(--f4)">
									<span>Lower Bound Edge</span>
									<span
										className={
											histLowerBound > 0
												? "text-(--up) font-bold"
												: "text-(--f3)"
										}
									>
										{histLowerBound > 0 ? "SURVIVES 0 BP" : "NO EDGE"}
									</span>
								</div>
								<div className="relative h-1.5 w-full bg-(--sunken) rounded-full overflow-hidden border border-(--line)">
									<div className="absolute left-1/2 top-0 bottom-0 w-0.5 bg-(--line2) z-10" />
									{histCorrectEnter + histFalseEnter > 1 &&
										histLowerBound !== 0 && (
											<div
												style={{
													left:
														histLowerBound > 0
															? "50%"
															: `${Math.max(0, 50 + histLowerBound * 500)}%`,
													width: `${Math.min(50, Math.abs(histLowerBound) * 500)}%`,
												}}
												className={`absolute top-0 bottom-0 ${
													histLowerBound > 0 ? "bg-(--up)" : "bg-(--down)"
												}`}
											/>
										)}
								</div>
							</div>

							<div className="flex justify-between items-baseline text-[10px] text-(--f3) pt-1 border-t border-(--line)">
								<span>
									Correct:{" "}
									<strong
										className="text-(--up)"
										data-metric="hist_correct_enter"
									>
										{histCorrectEnter}
									</strong>
								</span>
								<span>
									False:{" "}
									<strong
										className="text-(--down)"
										data-metric="hist_false_enter"
									>
										{histFalseEnter}
									</strong>
								</span>
								<span>
									Missed:{" "}
									<strong
										className="text-(--down)"
										data-metric="hist_missed_enter"
									>
										{histMissedEnter}
									</strong>
								</span>
							</div>
						</Section>

						<Section fit="content" className="border-(--line) border-b">
							<Section.Header
								size="s"
								title="Forward paper evidence"
								meta={
									<Badge
										size="xs"
										variant={isForward ? "success" : "disabled"}
										label={isForward ? "Active" : "Gated"}
									/>
								}
							/>

							{/* Visual Execution Fill Ratio Meter */}
							<div className="flex flex-col gap-1">
								<div className="h-1.5 w-full rounded-full bg-(--sunken) border border-(--line) overflow-hidden">
									<div
										style={{
											width: `${
												fwdPredictions > 0
													? Math.min(
															100,
															(fwdPaperTrades / fwdPredictions) * 100,
														)
													: 0
											}%`,
										}}
										className="h-full bg-(--info) transition-all duration-300"
									/>
								</div>
								<div className="flex justify-between text-[9px] text-(--f4)">
									<span>Execution Coverage</span>
									<span>
										{fwdPredictions > 0
											? `${Math.round((fwdPaperTrades / fwdPredictions) * 100)}%`
											: "0%"}
									</span>
								</div>
							</div>

							<div className="flex justify-between items-baseline text-[11px]">
								<span className="text-(--f4)">Paper Mean Return:</span>
								<span
									className="text-(--f1) font-bold"
									data-metric="fwd_paper_mean_return"
									data-format="insufficient_if_zero"
								>
									{fwdPaperTrades > 0 ? basis(fwdPaperMeanReturn) : "—"}
								</span>
							</div>
							<div className="flex justify-between items-baseline text-[10px]">
								<span className="text-(--f4)">Paper Lower Bound:</span>
								<span
									className="text-(--acc) font-bold"
									data-metric="fwd_paper_lower_bound"
									data-format="insufficient_if_zero"
								>
									{fwdPaperTrades > 0 ? basis(fwdPaperLowerBound) : "—"}
								</span>
							</div>

							<div className="flex justify-between items-baseline text-[10px] text-(--f3) pt-1 border-t border-(--line)">
								<span>
									Trades:{" "}
									<strong
										className="text-(--f1)"
										data-metric="fwd_paper_trades"
										data-format="integer"
									>
										{fwdPaperTrades}
									</strong>
								</span>
								<span>
									Predictions:{" "}
									<strong
										className="text-(--f2)"
										data-metric="fwd_enter_predictions"
										data-format="integer"
									>
										{fwdPredictions}
									</strong>
								</span>
							</div>
							<div className="flex justify-between items-baseline text-[10px] text-(--f4)">
								<span>Active Position:</span>
								<span data-l="paper-position" className="text-(--f2) font-bold">
									{openPositionsCount > 0
										? `${openPositionsCount} active`
										: "None"}
								</span>
							</div>
						</Section>

						<Section.Header
							size="s"
							title="Gate blocker"
							meta={
								<Typography.Mono
									size="s"
									tone="down"
									data-l="stage-blocker"
								>
									{stageBlocker || "—"}
								</Typography.Mono>
							}
						/>
					</Section.Body>
				</Section>
			</Flex>

			<Flex className="h-2/5 min-h-0 border-(--line) border-t">
				<Section fit="pane" className="min-w-0 flex-1 border-(--line) border-r">
					<Section.Header
						size="s"
						title="Radix trie memory"
						meta={`${trieBranches
							.reduce(
								(acc, p) => acc + (p.observed === false ? 0 : p.visits),
								0,
							)
							.toLocaleString()} observations routed`}
					/>
					<Section.Body>
						<table className="w-full text-left border-collapse text-[10px]">
							<thead>
								<tr className="text-(--f4) border-(--line) border-b">
									<th className="font-normal pb-1.5 px-2">Path Signature</th>
									<th className="font-normal pb-1.5 px-2 text-right">Depth</th>
									<th className="font-normal pb-1.5 px-2 text-right">Visits</th>
									<th className="font-normal pb-1.5 px-2 text-right">
										Policy Bias
									</th>
									<th className="font-normal pb-1.5 px-2">Confidence</th>
									<th className="font-normal pb-1.5 px-2 text-right">
										Learned Policy
									</th>
								</tr>
							</thead>
							<tbody>
								{trieBranches.map((p, i) => (
									<tr
										key={p.hash}
										className="border-(--line)/50 border-b last:border-0 hover:bg-(--raised)"
									>
										<td
											className={cn(
												"py-1.5 px-2 font-mono",
												i === 0 ? "text-(--acc)" : "text-(--f2)",
											)}
										>
											{p.hash}
											{p.observed === false ? (
												<span className="ml-1 text-(--f4)">derived</span>
											) : null}
										</td>
										<td className="py-1.5 px-2 text-right">{p.depth}</td>
										<td className="py-1.5 px-2 text-right">
											{p.observed === false ? "—" : p.visits.toLocaleString()}
										</td>
										<td
											className={cn(
												"py-1.5 px-2 text-right font-bold",
												p.observed === false
													? "text-(--f4)"
													: p.meanEdge >= 0
														? "text-(--up)"
														: "text-(--down)",
											)}
										>
											{p.observed === false
												? "unavailable"
												: `${p.meanEdge >= 0 ? "+" : ""}${(p.meanEdge * 100).toFixed(1)}%`}
										</td>
										<td className="py-1.5 px-2">
											{p.observed === false ? (
												<span className="text-(--f4)">unavailable</span>
											) : (
												<div className="flex items-center gap-2">
													<div className="w-12 h-1 bg-(--sunken) rounded overflow-hidden">
														<div
															className="h-full bg-(--info)"
															style={{ width: `${p.confidence}%` }}
														/>
													</div>
													<span>{p.confidence.toFixed(1)}%</span>
												</div>
											)}
										</td>
										<td className="py-1.5 px-2 text-right">
											<Badge
												size="xxs"
												variant={p.policy === "ENTER" ? "success" : "disabled"}
												label={p.policy}
											/>
										</td>
									</tr>
								))}
								{trieBranches.length === 0 && (
									<tr>
										<td colSpan={6} className="py-4 text-center text-(--f4)">
											No cognitive radix branches registered yet.
										</td>
									</tr>
								)}
							</tbody>
						</table>
					</Section.Body>
				</Section>

				<Section fit="pane" className="w-95 shrink-0">
					<Section.Header
						size="s"
						title="Edge distribution"
						meta={
							edgeSamples.length > 0
								? `${edgeSamples.length} samples`
								: evaluatedCount > 0
									? `${evaluatedCount} evaluations`
									: "Impartial"
						}
					/>
					<Section.Body scroll={false} className="relative">
						{edgeSamples.length === 0 ? (
							<div className="absolute inset-0 flex items-center justify-center text-(--f4) text-[10px] px-4 text-center">
								No return distribution is drawn. Mean and lower bound stay in
								the evidence panel.
							</div>
						) : (
							(() => {
								const neg = edgeSamples.filter((v) => v < 0).length;
								const zero = edgeSamples.filter((v) => v === 0).length;
								const pos = edgeSamples.filter((v) => v > 0).length;
								const total = Math.max(edgeSamples.length, 1);
								const bars = [
									{ label: "−1", count: neg, color: "var(--down)" },
									{ label: "0", count: zero, color: "var(--f4)" },
									{ label: "+1", count: pos, color: "var(--up)" },
								];
								const maxCount = Math.max(...bars.map((b) => b.count), 1);
								return (
									<svg
										viewBox="0 0 340 160"
										className="w-full h-full"
										aria-label="Observed skill return histogram"
									>
										<title>Observed skill return histogram</title>
										{bars.map((bar, index) => {
											const x = 40 + index * 100;
											const height = (bar.count / maxCount) * 110;
											const y = 130 - height;
											return (
												<g key={bar.label}>
													<rect
														x={x}
														y={y}
														width={60}
														height={height}
														fill={bar.color}
														opacity={0.85}
													/>
													<text
														x={x + 30}
														y={144}
														textAnchor="middle"
														fill="var(--f3)"
														fontSize="10"
													>
														{bar.label}
													</text>
													<text
														x={x + 30}
														y={y - 4}
														textAnchor="middle"
														fill="var(--f1)"
														fontSize="10"
													>
														{bar.count}
													</text>
												</g>
											);
										})}
										<text
											x={170}
											y={12}
											textAnchor="middle"
											fill="var(--f4)"
											fontSize="9"
										>
											{edgeSamples.length} observed skill outcomes ·{" "}
											{(100 * pos) / total}% correct
										</text>
									</svg>
								);
							})()
						)}
					</Section.Body>

					<Section.Header
						size="s"
						title={
							edgeSamples.length === 0
								? "A curve is omitted until completed outcomes supply their own return sample."
								: "Histogram of graded skill samples from supervise (correct=+1, incorrect=−1)."
						}
					/>
				</Section>
			</Flex>
		</Flex.Column>
	);
};
