import { useSelector } from "@tanstack/react-store";
import * as d3 from "d3";
import { ChevronRight, Clock, Pause, Play } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { focusAtom, positionCountAtom, type RingBuffer, signals } from "#/collections/app";
import { RingCursor } from "#/collections/ring";
import { Scanlines } from "#/components/ui/scanlines";
import { hubBaseUrl } from "#/lib/hub";
import { cn } from "#/lib/utils";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { basis, outcome, prediction } from "./format";
import type {
	CognitionTreeResponse,
	TrieBranch,
} from "./types";

interface ForwardTapePoint {
	x: number;
	y: number;
	seq?: number;
	time?: number;
}

interface TrainedFragmentResponse {
	id: number;
	symbol: string;
	epoch: number;
	mark_a: number;
	mark_b: number;
	mark_c: number;
	entry_price: number;
	exit_price: number;
	magnitude: number;
	direction: string;
	class?: string;
	tokens: string[];
	points: { x: number; y: number; seq: number; time: number }[];
	entry_idx: number;
	exit_idx: number;
	predicted_entry_idx: number;
	predicted_exit_idx: number;
	learned_at: string;
}

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
	if (time > 1e10) return Math.floor(time);       // milliseconds
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
function fragmentClass(className: string | undefined, direction: string): ExcursionKind {
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
function fragmentClassCode(className: string | undefined, direction: string): string {
	if (className) return className;
	return direction || "unknown";
}

function fragmentDelayedLabel(className: string | undefined, direction: string): string {
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

function fragmentFrozenAction(className: string | undefined, direction: string): string {
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

const getTargetRing = (
	records: Record<string, RingBuffer<MeasurementT>> | undefined,
	target: string,
): { ring: RingBuffer<MeasurementT>; symbol: string } | null => {
	if (!records) return null;
	if (target && records[target] && !records[target].isEmpty()) {
		return { ring: records[target], symbol: target };
	}
	if (records.learner && !records.learner.isEmpty()) {
		return { ring: records.learner, symbol: "learner" };
	}
	let bestRing: RingBuffer<MeasurementT> | null = null;
	let bestSym = "";
	let latestTick = -1n;

	for (const [sym, ring] of Object.entries(records)) {
		if (!ring || ring.isEmpty()) continue;
		const len = ring.getBufferLength();
		const last = len > 0 ? ring.get(len - 1) : undefined;
		const tick = last?.tick ?? 0n;
		if (tick >= latestTick || bestRing === null) {
			latestTick = tick;
			bestRing = ring;
			bestSym = sym;
		}
	}
	if (bestRing) {
		return { ring: bestRing, symbol: bestSym };
	}
	return null;
};

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
	const globalFocus = useSelector(focusAtom, (state) => state);
	const targetSymbol = propSymbol || globalFocus;
	const historicalOnly = tapeSource === "historical";
	const tapeRef = useRef<HTMLDivElement>(null);
	const [tapeDim, setTapeDim] = useState({ width: 800, height: 300 });
	const [isPlaying, setIsPlaying] = useState(true);

	// Real tape points: live stage stream and/or a pinned trained fragment.
	const [points, setPoints] = useState<ForwardTapePoint[]>([]);

	// Real stage & execution mode
	const [stageCode, setStageCode] = useState(0);
	const [stageName, setStageName] = useState("MODEL DEVELOPMENT");
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
	const [trieBranches, setTrieBranches] = useState<TrieBranch[]>([]);

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

	const [trainedFragments, setTrainedFragments] = useState<TrainedFragmentResponse[]>([]);
	const [selectedFragmentIndex, setSelectedFragmentIndex] = useState<number>(-1);
	/*
	 * pinnedFragmentRef freezes the tape chart while the operator inspects a
	 * historical run. Live hub measurements still update stage/side metrics,
	 * but must not rewrite points, symbol, or excursion markers on the chart.
	 */
	const pinnedFragmentRef = useRef(false);
	const openFragmentRef = useRef<number | null>(null);

	const loadFragment = useCallback((frag: TrainedFragmentResponse) => {
		if (!frag) return;
		pinnedFragmentRef.current = true;
		setCurrentEpisode(null);
		currentEpisodeRef.current = null;
		setCurrentSymbol(frag.symbol);
		const mappedPoints = (frag.points ?? []).map((pt, i) => ({
			x: i,
			y: pt.y,
			seq: pt.seq,
			time: toTimestampMs(pt.time),
		}));
		setPoints(mappedPoints);
		// The list carries no points: a fragment's stored trades are read from
		// the tape when it is opened.
		openFragmentRef.current = frag.id ?? null;
		if (mappedPoints.length === 0 && frag.id !== undefined) {
			void fetch(`${hubBaseUrl()}/training/fragments/${frag.id}/points`)
				.then((res) => (res.ok ? res.json() : []))
				.then((tape: TrainedFragmentResponse["points"]) => {
					if (!pinnedFragmentRef.current || openFragmentRef.current !== frag.id) return;
					setPoints(
						(tape ?? []).map((pt, i) => ({
							x: i,
							y: pt.y,
							seq: pt.seq,
							time: toTimestampMs(pt.time),
						})),
					);
				})
				.catch(() => {});
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
		if (selectedFragmentIndex < 0 || selectedFragmentIndex >= trainedFragments.length) return;
		const frag = trainedFragments[selectedFragmentIndex];
		const episodePoints = (frag.points ?? []).map((pt, i) => ({
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
	}, [selectedFragmentIndex, trainedFragments]);

	useEffect(() => {
		let isMounted = true;
		const fetchFragments = async () => {
			try {
				const res = await fetch(`${hubBaseUrl()}/training/fragments`);
				if (!res.ok) return;
				const frags: TrainedFragmentResponse[] = await res.json();
				if (!isMounted || !Array.isArray(frags)) return;
				setTrainedFragments(frags);
			} catch {
				// Endpoint connecting
			}
		};

		fetchFragments();
		const interval = setInterval(fetchFragments, 3000);
		return () => {
			isMounted = false;
			clearInterval(interval);
		};
	}, []);

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
		const unsubPos = positionCountAtom.subscribe((count) => {
			setOpenPositionsCount(count);
		});
		setOpenPositionsCount(positionCountAtom.get());

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

	// Fetch real Radix Trie tree & branch data from backend
	useEffect(() => {
		let isMounted = true;

		const fetchTree = async () => {
			try {
				const res = await fetch(`${hubBaseUrl()}/cognition/tree`);
				if (!res.ok) return;
				const data: CognitionTreeResponse = await res.json();
				if (!isMounted) return;

				if (data.branches && data.branches.length > 0) {
					setTrieBranches(data.branches);
				}
			} catch {
				// Backend endpoint connecting
			}
		};

		fetchTree();
		const interval = window.setInterval(fetchTree, 4000);
		return () => {
			isMounted = false;
			clearInterval(interval);
		};
	}, []);

	// A newly completed episode must wake playback even when the queue was empty.
	useEffect(() => {
		if (!isPlaying || currentEpisode || episodeQueue.length === 0) return;
		setCurrentEpisode(episodeQueue[0]);
		setPlaybackTick(0);
		setEpisodeQueue((queue) => queue.slice(1));
	}, [isPlaying, currentEpisode, episodeQueue]);

	// Playback Animation Loop
	useEffect(() => {
		if (!isPlaying) return;

		let timer: number;
		if (playbackPhase === "PLAYING") {
			if (!currentEpisode) {
				return;
			}

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
		} else if (playbackPhase === "EVALUATING") {
			timer = window.setTimeout(() => {
				setCurrentEpisode(null);
				setPlaybackPhase("PLAYING");
			}, 1200); // Wait 1.2s at the end of the excursion to show the result
		}

		return () => {
			clearInterval(timer);
			clearTimeout(timer);
		};
	}, [isPlaying, playbackPhase, currentEpisode]);

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
		const cursor = new RingCursor<MeasurementT>();
		pendingEpisodeRef.current = [];
		currentEpisodeRef.current = null;
		setEpisodeQueue([]);
		if (!historicalOnly && !pinnedFragmentRef.current) {
			setPoints([]);
			setExcursionEvent(null);
		}
		lastSeenExcursionStartRef.current = -1;

		const handleRing = (ring: RingBuffer<MeasurementT>) => {
			if (!ring || ring.isEmpty()) return;

			cursor.read(ring, (measurement) => {
				const activeSym = String(measurement.symbol || "TRAINING");
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

				const metricMap: Record<string, number> = {};
				for (const m of measurement.metrics ?? []) {
					if (!m?.name) continue;
					metricMap[String(m.name)] = m.raw ?? 0;
				}

				const evalCount = metricMap.evaluated ?? 0;
				setEvaluatedCount(Math.floor(evalCount));

				// Update stage
				const sCode = metricMap.stage_code ?? 0;
				setStageCode(sCode);
				let sName = "MODEL DEVELOPMENT";
				if (sCode === 1) sName = "HISTORICAL VALIDATION";
				if (sCode === 2) sName = "FORWARD PAPER LEARNING";
				if (sCode === 3) sName = "FORWARD SKILL DEMONSTRATED";

				if (measurement.provenance) {
					for (const p of measurement.provenance) {
						if (p?.name === "stage" && p.value) {
							sName = String(p.value);
						}
						if (p?.name === "stage_blocker" && p.value !== undefined) {
							setStageBlocker(String(p.value));
						}
					}
				}
				setStageName(sName);

				// Precursor & prediction
				const pLen = Math.floor(metricMap.precursor_length ?? 0);
				setPrecursorLength(pLen);

				let tokensList: string[] = [];
				let excDirection = "";
				if (measurement.provenance) {
					for (const p of measurement.provenance) {
						if (p?.name === "precursor_tokens" && p.value) {
							tokensList = String(p.value).split(",").filter(Boolean);
						}
						if (p?.name === "excursion_direction") excDirection = String(p.value);
					}
				}
				let excStart: number | undefined;
				let excIgnition: number | undefined;
				let excExit: number | undefined;

				if (measurement.metadata) {
					for (const m of measurement.metadata) {
						if (m?.name === "precursor_tokens" && m.value) {
							tokensList = String(m.value).split(",").filter(Boolean);
						}
						if (m?.name === "excursion_start" && m.value) {
							excStart = Number(m.value);
						}
						if (m?.name === "excursion_ignition" && m.value) {
							excIgnition = Number(m.value);
						}
						if (m?.name === "excursion_exit" && m.value) {
							excExit = Number(m.value);
						}
						if (m?.name === "excursion_direction" && m.value) {
							excDirection = String(m.value);
						}
					}
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

				if (measurement.provenance) {
					for (const m of measurement.provenance) {
						if (m?.name === "edge_samples" && m.value) {
							const parsed = String(m.value)
								.split(",")
								.map((part) => Number(part.trim()))
								.filter((value) => Number.isFinite(value));
							if (parsed.length > 0) {
								setEdgeSamples(parsed);
							}
						}
					}
				}

				// Forward paper
				setFwdPredictions(Math.floor(metricMap.fwd_enter_predictions ?? 0));
				setFwdPaperTrades(Math.floor(metricMap.fwd_paper_trades ?? 0));
				setFwdPaperMeanReturn(metricMap.fwd_paper_mean_return ?? 0);
				setFwdPaperLowerBound(metricMap.fwd_paper_lower_bound ?? 0);

				// Extract price directly from measurement or metrics (which is log-return)
				let rawPrice: number | undefined;
				if (metricMap.price !== undefined) {
					rawPrice = metricMap.price;
				} else if (measurement.metrics) {
					for (const [k, met] of Object.entries(measurement.metrics)) {
						if (met?.raw !== undefined) {
							const lk = k.toLowerCase();
							const validPrices = [
								"price",
								"last_price",
								"mark_price",
								"best_bid_price",
								"best_ask_price",
								"trade_price",
								"mid",
								"close",
							];
							if (validPrices.includes(lk)) {
								rawPrice = met.raw;
								break;
							}
						}
					}
				}

				if (rawPrice === undefined && measurement.peers) {
					for (const peer of measurement.peers) {
						for (const m of peer.metrics ?? []) {
							if (m?.name && m?.raw !== undefined) {
								const nm = String(m.name).toLowerCase();
								const validPrices = [
									"price",
									"last_price",
									"mark_price",
									"best_bid_price",
									"best_ask_price",
									"trade_price",
									"mid",
									"close",
								];
								if (validPrices.includes(nm)) {
									rawPrice = m.raw;
									break;
								}
							}
						}
						if (rawPrice !== undefined) break;
					}
				}

				if (rawPrice !== undefined) {
					const seqVal = Number(measurement.tick ?? 0n);
					const pointPrice = rawPrice;
					const ptTime = parseTimestamp(
						measurement.at,
						measurement.observedFrom,
					);
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

				// String event tags are carried by the wire's provenance vector.
				if (measurement.provenance) {
					for (const m of measurement.provenance) {
						if (
							m?.name === "excursion_event" &&
							String(m.value) === "completed"
						) {
							if (pendingEpisodeRef.current.length > 0) {
								const readyEpisode = [...pendingEpisodeRef.current];
								setEpisodeQueue((q) => [...q, readyEpisode].slice(-50));
							}
							pendingEpisodeRef.current = [];
						}
					}
				}
			});
		};

		const updateFromState = (
			state: Record<string, RingBuffer<MeasurementT>> | undefined,
		) => {
			if (!state) return;
			const target = getTargetRing(state, targetSymbol);
			if (target) {
				handleRing(target.ring);
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
		const count = Math.max(points.length, 50);
		const xs = d3.scaleLinear().domain([0, count]).range([0, tapeDim.width]);

		let minY = Number.POSITIVE_INFINITY;
		let maxY = Number.NEGATIVE_INFINITY;
		for (const pt of points) {
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

		// Calculate time ticks and span across evaluated tape fragment
		const validTimes = points
			.map((p) => p.time)
			.filter((t): t is number => typeof t === "number" && t > 0);

		let timeSpanLabel = "";
		const timeTicks: Array<{ x: number; label: string }> = [];

		if (validTimes.length >= 2 && tapeDim.width > 0 && points.length > 0) {
			const tStart = validTimes[0];
			const tEnd = validTimes[validTimes.length - 1];
			const spanMs = Math.max(0, tEnd - tStart);
			timeSpanLabel = formatDurationSpan(spanMs);

			const activeWidth = xs(points.length - 1);
			const numTicks = Math.max(2, Math.min(6, Math.floor(activeWidth / 130)));

			const tickIndices = Array.from({ length: numTicks }, (_, i) =>
				Math.min(
					points.length - 1,
					Math.round((i / Math.max(1, numTicks - 1)) * (points.length - 1)),
				),
			);

			const times = tickIndices.map((idx) => {
				const ptTime = points[idx]?.time;
				if (ptTime && ptTime > 0) return ptTime;
				const fraction = idx / Math.max(1, points.length - 1);
				return tStart + fraction * spanMs;
			});

			const testLabels = times.map((t) => formatClockTime(t, false));
			const needsSeconds =
				spanMs < 120_000 || new Set(testLabels).size < testLabels.length;

			for (let i = 0; i < numTicks; i++) {
				const ptIdx = tickIndices[i];
				const px = xs(points[ptIdx].x);
				const clock = formatClockTime(times[i], needsSeconds);
				const relSec = ((times[i] - tStart) / 1000).toFixed(spanMs < 10_000 ? 1 : 0);
				const label = i === 0 ? clock : `${clock} (+${relSec}s)`;
				timeTicks.push({ x: px, label });
			}
		} else if (points.length >= 2 && tapeDim.width > 0) {
			const seqs = points.map((p) => p.seq).filter((s): s is number => typeof s === "number" && s > 0);
			if (seqs.length >= 2) {
				const numTicks = Math.max(2, Math.min(5, Math.floor(tapeDim.width / 140)));
				for (let i = 0; i < numTicks; i++) {
					const idx = Math.min(
						points.length - 1,
						Math.round((i / Math.max(1, numTicks - 1)) * (points.length - 1)),
					);
					const px = xs(points[idx].x);
					timeTicks.push({ x: px, label: `tick #${points[idx].seq ?? idx}` });
				}
			}
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
	}, [points, tapeDim.width, tapeDim.height]);

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
		<div className="flex flex-col w-full h-full gap-2 font-mono text-[11px] bg-(--bg) p-2 overflow-hidden text-(--f2)">
			{/* TOP ROW: optional fragment rail + Real Tape + Right Sidebar */}
			<div className="flex h-3/5 gap-2 min-h-0">
				{historicalOnly && (
					<div
						className="w-56 bg-(--surface) border-(--line) border rounded flex flex-col shrink-0 min-h-0"
						data-l="historical-runs-list"
					>
						<div className="h-8 border-(--line) border-b bg-(--sunken) flex items-center px-3 text-(--f3) shrink-0 justify-between">
							<span className="tracking-widest uppercase font-bold text-[10px]">
								Historical Runs
							</span>
							<span className="text-[10px] text-(--f4)">
								{trainedFragments.length}
							</span>
						</div>
						<div className="flex-1 overflow-y-auto min-h-0">
							{trainedFragments.length === 0 ? (
								<div className="p-3 text-(--f4) text-[10px] leading-relaxed">
									No detections read yet. Train lists every stored detection at
									startup (/training/fragments).
								</div>
							) : (
								trainedFragments
									.map((frag, idx) => ({ frag, idx }))
									.reverse()
									.map(({ frag, idx }) => {
										const active = idx === selectedFragmentIndex;
										const cls = fragmentClassCode(frag.class, frag.direction);
										return (
											<button
												key={frag.id ?? idx}
												type="button"
												onClick={() => selectFragment(idx)}
												className={cn(
													"w-full text-left px-2.5 py-2 border-b border-(--line) cursor-pointer transition-colors",
													active
														? "bg-(--acc)/10 text-(--acc)"
														: "hover:bg-(--sunken) text-(--f2)",
												)}
												data-l={active ? "historical-run-active" : undefined}
											>
												<div className="flex items-center justify-between gap-2">
													<span className="font-bold text-[10px] truncate">
														#{frag.id} {frag.symbol}
													</span>
													<span className="text-[9px] uppercase tracking-wider shrink-0 opacity-80">
														[{cls}]
													</span>
												</div>
												<div className="mt-0.5 text-[9px] text-(--f4) flex justify-between gap-2">
													<span>{(frag.direction || "").toUpperCase()}</span>
													<span>{(frag.magnitude * 100).toFixed(2)}%</span>
												</div>
											</button>
										);
									})
							)}
						</div>
					</div>
				)}
				{/* Main Episodic Tape */}
				<div className="flex-1 bg-(--sunken) border-(--line) border rounded flex flex-col relative min-w-0 shadow-[inset_0_0_24px_rgba(0,0,0,0.85)]">
					{/* Tape Header */}
					<div className="h-8 border-(--line) border-b bg-(--sunken) flex items-center px-3 justify-between text-(--f3) shrink-0">
						<div className="flex items-center gap-2 min-w-0">
							<span
								data-l="tape-title"
								className={cn(
									"font-bold px-1.5 py-0.5 rounded text-[10px] shrink-0",
									historicalOnly || selectedFragmentIndex >= 0
										? "bg-(--info)/10 text-(--info) border border-(--info)/30"
										: stageCode === 1
											? "bg-(--acc)/10 text-(--acc) border border-(--acc)/30"
											: "bg-(--info)/10 text-(--info) border border-(--info)/30",
								)}
							>
								{historicalOnly
									? "HISTORICAL RUNS"
									: selectedFragmentIndex >= 0
										? "TRAINED FRAGMENTS"
										: isForward
											? "TRAINED FRAGMENTS"
											: "HISTORICAL REPLAY TAPE"}
							</span>
							<span className="bg-(--surface) border-(--line) border px-1.5 py-0.5 rounded text-[10px] text-(--f1) font-bold shrink-0">
								{currentSymbol}
							</span>

							{trainedFragments.length > 0 && (
								<div className="flex items-center gap-1 ml-1 min-w-0" data-l="fragment-selector">
									<button
										type="button"
										onClick={() => selectFragment(selectedFragmentIndex - 1)}
										disabled={selectedFragmentIndex <= 0}
										className="px-1.5 py-0.5 rounded bg-(--surface) border border-(--line) text-(--f2) hover:text-(--f1) disabled:opacity-30 disabled:pointer-events-none text-[10px] cursor-pointer"
										title="Previous fragment"
									>
										◀
									</button>
									<select
										value={selectedFragmentIndex}
										onChange={(e) => selectFragment(Number(e.target.value))}
										className="bg-(--surface) border border-(--line) text-(--f1) text-[10px] px-1.5 py-0.5 rounded focus:outline-none focus:border-(--acc) cursor-pointer max-w-[280px]"
										aria-label="Select historical training run"
									>
										{!historicalOnly && selectedFragmentIndex < 0 && (
											<option value={-1}>Live stream…</option>
										)}
										{trainedFragments.map((frag, idx) => (
											<option key={frag.id ?? idx} value={idx}>
												#{frag.id} {frag.symbol} [{fragmentClassCode(frag.class, frag.direction)}] ({frag.direction.toUpperCase()} {(frag.magnitude * 100).toFixed(2)}%)
											</option>
										))}
									</select>
									<button
										type="button"
										onClick={() => selectFragment(selectedFragmentIndex + 1)}
										disabled={selectedFragmentIndex >= trainedFragments.length - 1}
										className="px-1.5 py-0.5 rounded bg-(--surface) border border-(--line) text-(--f2) hover:text-(--f1) disabled:opacity-30 disabled:pointer-events-none text-[10px] cursor-pointer"
										title="Next fragment"
									>
										▶
									</button>
									<button
										type="button"
										onClick={replayFragment}
										disabled={selectedFragmentIndex < 0}
										className="ml-1 px-1.5 py-0.5 rounded bg-(--acc)/10 text-(--acc) border border-(--acc)/30 hover:bg-(--acc)/20 disabled:opacity-30 disabled:pointer-events-none text-[10px] font-bold cursor-pointer"
										title="Replay this fragment animation"
									>
										Replay
									</button>
									{!historicalOnly && selectedFragmentIndex >= 0 && (
										<button
											type="button"
											onClick={resumeLiveTape}
											className="px-1.5 py-0.5 rounded bg-(--surface) border border-(--line) text-(--f2) hover:text-(--acc) text-[10px] font-bold cursor-pointer"
											title="Return to live training tape"
											data-l="resume-live-tape"
										>
											Live
										</button>
									)}
								</div>
							)}

							<ChevronRight className="w-3 h-3 text-(--f4)" />
							<span className="text-(--f1)">
								{points.length} frames evaluated
							</span>
							{timeSpanLabel && (
								<>
									<ChevronRight className="w-3 h-3 text-(--f4)" />
									<span
										className="text-(--acc) flex items-center gap-1 font-mono text-[10px] font-bold bg-(--acc)/10 px-1.5 py-0.5 rounded border border-(--acc)/20"
										title="Time-scale span of this tape fragment"
									>
										<Clock className="w-3 h-3 text-(--acc)" />
										<span>{timeSpanLabel}</span>
									</span>
								</>
							)}
						</div>
						<div className="flex items-center gap-3">
							{currentPoints.length > 0 && (
								<span className="text-[11px] font-bold text-(--acc) bg-(--acc)/10 px-2 py-0.5 rounded border border-(--acc)/20">
									Δ
									{currentPoints[currentPoints.length - 1].y.toLocaleString(
										undefined,
										{
											minimumFractionDigits: 5,
											maximumFractionDigits: 5,
										},
									)}
								</span>
							)}
							<span className="text-[10px] uppercase tracking-widest text-(--f4)">
								STAGE:{" "}
								<span
									data-l="training-stage"
									className="border-(--line) border text-(--f2) px-1 rounded ml-1 font-bold"
								>
									{stageName}
								</span>
							</span>
							<button
								type="button"
								onClick={() => {
									if (!isPlaying && !currentEpisode && selectedFragmentIndex >= 0 && selectedFragmentIndex < trainedFragments.length) {
										replayFragment();
									} else {
										setIsPlaying(!isPlaying);
									}
								}}
								className="text-(--acc) hover:text-(--f1) transition-colors p-1 cursor-pointer"
								title={isPlaying ? "Pause tape" : "Resume / replay tape"}
							>
								{isPlaying ? (
									<Pause className="w-3.5 h-3.5" />
								) : (
									<Play className="w-3.5 h-3.5" />
								)}
							</button>
						</div>
					</div>

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
					<div ref={tapeRef} className="flex-1 relative overflow-hidden bg-(--sunken)">
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
										{maxPrice.toLocaleString(undefined, { minimumFractionDigits: 5, maximumFractionDigits: 5 })}
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
										{midPrice.toLocaleString(undefined, { minimumFractionDigits: 5, maximumFractionDigits: 5 })}
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
										{minPrice.toLocaleString(undefined, { minimumFractionDigits: 5, maximumFractionDigits: 5 })}
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
										<circle r={7} fill="none" stroke="var(--acc)" strokeWidth="0.8" opacity="0.4" />
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
											if (val === null || val === undefined || val < 0 || points.length === 0)
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
											// 3. Nearest sequence index match within sequence range
											if (points.length > 0) {
												const minSeq = points[0].seq ?? 0;
												const maxSeq = points[points.length - 1].seq ?? 0;
												if (val >= minSeq && val <= maxSeq) {
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
																stroke="#0ea5e9"
																strokeWidth="1"
																strokeDasharray="2 4"
																opacity="0.35"
															/>
															<rect
																x={-7}
																y={8}
																width={14}
																height={14}
																fill="#050505"
																stroke="#0ea5e9"
																strokeWidth="1"
															/>
															<text
																x={0}
																y={18}
																fill="#0ea5e9"
																fontSize="9px"
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
															<circle r={4} fill="#22c55e" />
															<text
																x={6}
																y={-6}
																fill="#22c55e"
																fontSize="9px"
																fontWeight="bold"
															>
																ENTER
															</text>
															<line
																y2={tapeDim.height}
																stroke="#22c55e"
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
																stroke="#4ade80"
																strokeWidth={2}
																strokeDasharray="3 2"
															/>
															<text
																x={8}
																y={12}
																fill="#4ade80"
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
																fill="#050505"
																stroke="#22c55e"
																strokeWidth={0.5}
															/>
															<text
																x={6}
																y={-1}
																fill="#22c55e"
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
															<circle r={4} fill="#ef4444" />
															<text
																x={6}
																y={-6}
																fill="#ef4444"
																fontSize="9px"
																fontWeight="bold"
															>
																EXIT
															</text>
															<line
																y2={tapeDim.height}
																stroke="#ef4444"
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
																stroke="#f87171"
																strokeWidth={2}
																strokeDasharray="3 2"
															/>
															<text
																x={8}
																y={12}
																fill="#f87171"
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
										{timeTicks.map((tick, idx) => (
											<g key={`time-tick-${idx}-${tick.x}-${tick.label}`}>
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
													textAnchor={
														idx === 0
															? "start"
															: idx === timeTicks.length - 1
																? "end"
																: "middle"
													}
													dx={idx === 0 ? 4 : idx === timeTicks.length - 1 ? -4 : 0}
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
							<span data-l="fragment-class-detail" className="font-bold text-(--f1) uppercase">
								{excursionEvent?.classCode ?? "—"}
							</span>
							<span className="text-(--f4)">Boundaries:</span>
							<span>A: {excursionEvent?.marks.A ?? 0}</span>
							<span>→</span>
							<span>B: {excursionEvent?.marks.B ?? 0}</span>
							<span>→</span>
							<span>C: {excursionEvent?.marks.C ?? 0}</span>
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
				</div>

				{/* Right Sidebar: Separated Historical Held-Out & Forward Paper Evidence */}
				<div className="w-72 bg-(--surface) border-(--line) border rounded flex flex-col shrink-0 min-h-0">
					<div className="h-8 border-(--line) border-b bg-(--sunken) flex items-center px-3 text-(--f3) shrink-0 justify-between">
						<span className="tracking-widest uppercase font-bold text-[10px]">
							{isForward ? "Forward Paper Authority" : "Historical Validation"}
						</span>
						<span className="text-[10px]">
							{evaluatedCount.toLocaleString()} frames
						</span>
					</div>

					<div className="p-3 flex-1 overflow-y-auto flex flex-col gap-3">
						{/* Historical Held-Out Evidence Card */}
						<div className="border border-(--line) p-2.5 rounded bg-(--bg) flex flex-col gap-1.5">
							<div className="uppercase tracking-widest text-(--f4) text-[9px] font-bold flex justify-between">
								<span>Historical Prediction Outcomes</span>
								<span className="text-(--f2)">{histOpportunities} opps</span>
							</div>
							<div className="flex justify-between items-baseline text-[11px]">
								<span className="text-(--f4)">Mean Quote Return:</span>
								<span
									className="text-(--f1) font-bold"
									data-metric="hist_mean_return"
									data-format="insufficient_if_zero"
								>
									{histCorrectEnter + histFalseEnter > 0 ? basis(histMeanReturn) : "—"}
								</span>
							</div>
							<div className="flex justify-between items-baseline text-[10px]">
								<span className="text-(--f4)">Mean − Standard Error:</span>
								<span
									className="text-(--acc) font-bold"
									data-metric="hist_lower_bound"
									data-format="insufficient_if_zero"
								>
									{histCorrectEnter + histFalseEnter > 1 ? basis(histLowerBound) : "—"}
								</span>
							</div>
							<div className="flex justify-between items-baseline text-[10px] text-(--f3)">
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
						</div>

						{/* Forward Paper Evidence Card */}
						<div className="border border-(--line) p-2.5 rounded bg-(--bg) flex flex-col gap-1.5">
							<div className="uppercase tracking-widest text-(--f4) text-[9px] font-bold flex justify-between">
								<span>Forward Paper Evidence</span>
								<span
									className={
										isForward ? "text-(--up) font-bold" : "text-(--f4)"
									}
								>
									{isForward ? "ACTIVE" : "GATED"}
								</span>
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
							<div className="flex justify-between items-baseline text-[10px] text-(--f3)">
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
						</div>

						{/* Stage Gate Status */}
						<div className="text-[10px] border border-(--line) p-2 rounded bg-(--sunken) flex flex-col gap-1">
							<div className="flex justify-between font-bold">
								<span className="text-(--f4) uppercase">Gate Blocker:</span>
								<span
									data-l="stage-blocker"
									className="text-(--down) truncate max-w-32.5"
								>
									{stageBlocker || "—"}
								</span>
							</div>
						</div>
					</div>
				</div>
			</div>

			{/* BOTTOM ROW: Real Radix Trie Memory Table + Edge Distribution */}
			<div className="flex h-2/5 gap-2 min-h-0">
				{/* Radix Trie Memory Table */}
				<div className="flex-1 bg-(--surface) border-(--line) border rounded flex flex-col min-w-0">
					<div className="h-8 border-(--line) border-b bg-(--sunken) flex items-center px-3 justify-between shrink-0">
						<span className="text-(--f3) uppercase tracking-widest text-[10px] font-bold">
							Radix Trie Memory (Active Learned Branches)
						</span>
						<span className="text-(--f4) text-[10px]">
							{trieBranches
								.reduce((acc, p) => acc + p.visits, 0)
								.toLocaleString()}{" "}
							observations routed
						</span>
					</div>

					<div className="flex-1 overflow-auto p-2">
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
										</td>
										<td className="py-1.5 px-2 text-right">{p.depth}</td>
										<td className="py-1.5 px-2 text-right">
											{p.visits.toLocaleString()}
										</td>
										<td
											className={cn(
												"py-1.5 px-2 text-right font-bold",
												p.meanEdge >= 0 ? "text-(--up)" : "text-(--down)",
											)}
										>
											{p.meanEdge >= 0 ? "+" : ""}
											{(p.meanEdge * 100).toFixed(1)}%
										</td>
										<td className="py-1.5 px-2">
											<div className="flex items-center gap-2">
												<div className="w-12 h-1 bg-(--sunken) rounded overflow-hidden">
													<div
														className="h-full bg-(--info)"
														style={{ width: `${p.confidence}%` }}
													/>
												</div>
												<span>{p.confidence.toFixed(1)}%</span>
											</div>
										</td>
										<td className="py-1.5 px-2 text-right">
											<span
												className={cn(
													"px-1.5 py-0.5 rounded text-[8px] border font-bold",
													p.policy === "ENTER"
														? "bg-(--up)/10 border-(--up)/30 text-(--up)"
														: "bg-(--line2) border-(--line) text-(--f3)",
												)}
											>
												{p.policy}
											</span>
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
					</div>
				</div>

				{/* Edge Distribution Panel */}
				<div className="w-95 bg-(--surface) border-(--line) border rounded flex flex-col shrink-0 min-h-0">
					<div className="h-8 border-(--line) border-b bg-(--sunken) flex items-center px-4 shrink-0 justify-between">
						<div className="flex gap-4 uppercase tracking-widest text-[10px] font-bold">
							<span className="text-(--f4)">Model</span>
							<span className="text-(--acc)">Edge Distribution</span>
						</div>
						<span className="text-[10px] text-(--f4)">
							{edgeSamples.length > 0
								? `${edgeSamples.length} samples`
								: evaluatedCount > 0
									? `${evaluatedCount} evaluations`
									: "Impartial"}
						</span>
					</div>

					<div className="flex-1 relative overflow-hidden p-3">
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
					</div>

					<div className="p-2.5 border-t border-(--line) text-[10px] text-(--f4) leading-tight">
						{edgeSamples.length === 0
							? "A curve is omitted until completed outcomes supply their own return sample."
							: "Histogram of graded skill samples from supervise (correct=+1, incorrect=−1). Not invented."}
					</div>
				</div>
			</div>
		</div>
	);
};
