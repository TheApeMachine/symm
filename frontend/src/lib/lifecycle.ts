import { useEffect, useSyncExternalStore } from "react";
import { signals as appSignals } from "#/collections/app";
import type { DynamicStoploss } from "#/collections/types";
import { positionReadout } from "#/components/book-readout";
import { positionRows } from "#/components/learning/open-positions";
import {
	lifecycleFromDeskClosed,
	lifecycleFromDeskOpen,
} from "#/lib/desk-closed-lifecycle";
import { trySendCommand } from "#/lib/hub-socket";
import { syncOpenPositionCount } from "#/lib/open-position-count";

/*
Paper round trips as the dashboard run lives them, streamed over the hub as
position/<event>/<symbol> Values (data.PositionOrigin, strategy.PositionFrame
in strategy/position.go): one frame per event (the entry match, sizing, an
order submitted or failed, a fill, the exit match, the close) and one mark per
tick of an open position against the live bid. Times are RFC 3339 strings;
durations are nanoseconds.

Paper trading has no real-exchange ledger. Nothing here carries a venue
number: the view states those as not available instead of showing a zero.
*/
export const POSITION_ORIGIN = "position";
export const LAYER_POSITION = "position";

export type LifeEvent = {
	at: string;
	kind: string;
	detail: string;
	fields?: Record<string, unknown>;
};

export type LifeMark = {
	at: string;
	bid: number;
	quantity: number;
	entry_price: number;
	unrealized: number;
	unrealized_return: number;
};

export type LifeOutcome = {
	/** Return as a fraction (−0.024 = −2.4%). Used for win-rate math. */
	pnl: number;
	/** Same return in percent points (−2.4). */
	pnl_pct: number;
	/** Quote-currency P&L when the desk published it (USD lost/made). */
	money?: number | null;
	hold_ns: number;
	expected_hold_ns: number | null;
	triggers: string[] | null;
	/** Realized fee-net return captured at exit (fraction, not %). */
	realized?: number | null;
	/** Mark-to-market return at settle (fraction). The realized-vs-mark gap is the seam audit. */
	mark?: number | null;
	/** Repress events the lot held (tape timebase). */
	bars_held?: number | null;
	/** Empirical winning horizon in bars. */
	max_lifetime_bars?: number | null;
};

/*
LearnedExitPolicy is the Causal-MCTS stopping boundary the desk has tuned for
this symbol, copied onto the lot at entry (broker/exitpolicy.go teachPolicy).
A zero or absent value means the desk has not learned that boundary yet — the
stop falls back to the friction floor. The UI must distinguish "unlearned" from
"zero", so each field is a nullable percent, not a defaulted 0.
*/
export type LearnedExitPolicy = {
	/** Trailing giveback (percent) off the peak, once green. */
	trailing?: number | null;
	/** Invalidation cut (percent) below entry. */
	cut?: number | null;
	/** Empirical winning horizon in reprice bars. */
	maxBars?: number | null;
	/** Worst return (percent) ever realized on the symbol. */
	worst?: number | null;
};

/** Live DynamicStoploss snapshot copied onto lifecycle rows from desk lots. */
export type LiveStopState = DynamicStoploss;

export type Lifecycle = {
	id: string;
	symbol: string;
	status: string;
	policy_epoch?: string;
	/** Grammar path the desk opened the lot on (path discovery ledger). */
	path?: string;
	opened_at: string;
	closed_at?: string | null;
	events: LifeEvent[] | null;
	mark?: LifeMark | null;
	/** Live DynamicStoploss snapshot from the desk lot. */
	stop_state?: LiveStopState | null;
	/** Learned exit policy for this symbol (Causal MCTS), copied on at entry. */
	policy?: LearnedExitPolicy | null;
	/** Peak mark-to-market return (percent) the lot has reached. */
	peak_pct?: number | null;
	/** Fee-net realized return captured at exit (fraction). */
	realized_pct?: number | null;
	/** Net percent path, one mark per reprice — the episode the exit policy learns from. */
	pnl_trace?: number[] | null;
	/** Repress events held (tape timebase). */
	bars_held?: number | null;
	/** Human-readable reason the exit fired. */
	exit_trigger?: string | null;
	/** Exit mark at official close, percent. */
	official_exit_pct?: number | null;
	/** Bid marks sampled while the lot was open (for hold charts). */
	marks?: LifeMark[];
	outcome?: LifeOutcome | null;
	book?: string;
	path_found?: boolean;
	/** Set when this row was copied from a desk lot snapshot. */
	desk?: boolean;
	/** Set when this closed row was restored from S3 history. */
	history?: boolean;
	/** Tape ticks around the round trip: A before entry, B entry, C exit. */
	marks_a?: number | null;
	marks_b?: number | null;
	marks_c?: number | null;
};

export type Performance = {
	closed: number;
	wins: number;
	win_rate: number | null;
	mean_return: number | null;
	pnl: number;
};

export type LifecycleReport = {
	lifecycles: Lifecycle[];
	performance: Performance;
	open_count: number;
};

type PositionFrame = {
	id: string;
	symbol: string;
	status: string;
	policy_epoch: string;
	opened_at: string;
	closed_at: string | null;
	event: LifeEvent | null;
	mark: LifeMark | null;
	outcome: LifeOutcome | null;
	stop_state?: LiveStopState | null;
};

type Snapshot = { report: LifecycleReport };

const SESSION_LIFECYCLE_LIMIT = 200;
const HISTORY_LIFECYCLE_CAP = 10_000;
const lives = new Map<string, Lifecycle>();
const listeners = new Set<() => void>();

type HistoryStamp = {
	epoch: bigint;
	tick: bigint;
	timestamp: bigint;
};

const historyStamp = new Map<string, HistoryStamp>();

const countsTowardPerformance = (life: Lifecycle): boolean =>
	(life.book ?? "paper") !== "virtual" || life.path_found === true;

const summarize = (all: Lifecycle[]): LifecycleReport => {
	const closed = all.filter(
		(life) =>
			life.status === "closed" && life.outcome && countsTowardPerformance(life),
	);
	const returns = closed.map((life) => (life.outcome as LifeOutcome).pnl);
	const wins = closed.filter((life) => (life.outcome as LifeOutcome).pnl > 0);

	return {
		lifecycles: all,
		open_count: all.filter(
			(life) => life.status !== "closed" && life.status !== "abandoned",
		).length,
		performance: {
			closed: closed.length,
			wins: wins.length,
			win_rate: closed.length > 0 ? wins.length / closed.length : null,
			mean_return:
				closed.length > 0
					? returns.reduce((sum, value) => sum + value, 0) / closed.length
					: null,
			pnl: returns.reduce((sum, value) => sum + value, 0),
		},
	};
};

let snapshot: Snapshot = { report: summarize([]) };

const publish = () => {
	// Newest round trip first.
	const all = [...lives.values()].sort((a, b) =>
		b.opened_at.localeCompare(a.opened_at),
	);
	snapshot = { report: summarize(all) };
	syncOpenPositionCount();
	for (const listener of listeners) listener();
};

const isSnapshot = (frame: unknown): frame is Lifecycle => {
	if (!frame || typeof frame !== "object") return false;
	const value = frame as Record<string, unknown>;
	return (
		typeof value.id === "string" &&
		typeof value.symbol === "string" &&
		typeof value.status === "string" &&
		typeof value.opened_at === "string" &&
		(Array.isArray(value.events) || value.events === null)
	);
};

const deskLifeChanged = (
	prev: Lifecycle | undefined,
	next: Lifecycle,
): boolean => {
	if (!prev) return true;
	if (prev.status !== next.status) return true;
	if (prev.closed_at !== next.closed_at) return true;
	if (prev.mark?.bid !== next.mark?.bid) return true;
	if (prev.mark?.unrealized_return !== next.mark?.unrealized_return)
		return true;
	if (prev.outcome?.pnl !== next.outcome?.pnl) return true;
	if (prev.outcome?.pnl_pct !== next.outcome?.pnl_pct) return true;
	if ((prev.marks?.length ?? 0) !== (next.marks?.length ?? 0)) return true;
	if ((prev.events?.length ?? 0) !== (next.events?.length ?? 0)) return true;
	return false;
};

const storeLife = (life: Lifecycle): boolean => {
	const prior = lives.get(life.id);
	if (prior && !deskLifeChanged(prior, life)) {
		return false;
	}

	lives.delete(life.id);
	lives.set(life.id, life);

	const sessionCount = () =>
		[...lives.values()].filter((entry) => !entry.history).length;
	const historyCount = () =>
		[...lives.values()].filter((entry) => entry.history).length;

	const evictClosed = (historyOnly: boolean) => {
		for (const [id, entry] of lives) {
			if (historyOnly !== !!entry.history) continue;
			if (entry.status === "closed" || entry.status === "abandoned") {
				lives.delete(id);
				historyStamp.delete(id);
				return true;
			}
		}
		return false;
	};

	while (!life.history && sessionCount() > SESSION_LIFECYCLE_LIMIT) {
		if (!evictClosed(false)) break;
	}

	while (life.history && historyCount() > HISTORY_LIFECYCLE_CAP) {
		if (!evictClosed(true)) break;
	}

	return true;
};

const remember = (life: Lifecycle) => {
	if (storeLife(life)) {
		publish();
	}
};

const isFrame = (frame: unknown): frame is PositionFrame => {
	if (!frame || typeof frame !== "object") return false;
	const value = frame as Record<string, unknown>;
	return (
		typeof value.id === "string" &&
		typeof value.symbol === "string" &&
		typeof value.status === "string" &&
		typeof value.opened_at === "string" &&
		(value.event === null) !== (value.mark === null)
	);
};

/*
ingestPositionFrame folds one position frame into its round trip. A frame
that is not a PositionFrame (exactly one of event or mark) is reported and
dropped.
*/
export const ingestPositionFrame = (frame: unknown) => {
	if (isSnapshot(frame)) {
		remember({ ...frame, events: frame.events ?? [] });
		return;
	}

	if (!isFrame(frame)) {
		console.error("ingestPositionFrame: malformed position frame", frame);
		return;
	}

	const life: Lifecycle = lives.get(frame.id) ?? {
		id: frame.id,
		symbol: frame.symbol,
		status: frame.status,
		policy_epoch: frame.policy_epoch,
		opened_at: frame.opened_at,
		events: [],
	};

	let marks = life.marks ?? [];

	if (frame.mark) {
		const last = marks[marks.length - 1];
		if (!last || last.at !== frame.mark.at || last.bid !== frame.mark.bid) {
			marks = [...marks, frame.mark].slice(-400);
		}
	}

	const next: Lifecycle = {
		...life,
		status: frame.status,
		closed_at: frame.closed_at ?? life.closed_at,
		outcome: frame.outcome ?? life.outcome,
		mark: frame.mark ?? life.mark,
		stop_state: frame.stop_state ?? life.stop_state,
		marks: marks.length > 0 ? marks : life.marks,
		events: frame.event ? [...(life.events ?? []), frame.event] : life.events,
	};

	remember(next);
};

/*
ingestDeskClosedLots mirrors the desk snapshot. A lot is open when it is in
that snapshot without a close, and closed when Exit put a close on it. Desk
rows that are no longer open are removed. A close already recorded stays.
*/
export const ingestDeskClosedLots = (rows: Record<string, unknown>[]) => {
	const openIds = new Set(
		rows
			.filter((row) => row.id && !positionReadout(row).closed)
			.map((row) => String(row.id)),
	);

	let changed = false;

	for (const [id, life] of lives) {
		if (!life.desk || life.status === "closed") {
			continue;
		}

		if (!openIds.has(id)) {
			lives.delete(id);
			changed = true;
		}
	}

	for (const row of rows) {
		const life =
			lifecycleFromDeskClosed(row as Record<string, unknown>) ??
			lifecycleFromDeskOpen(row as Record<string, unknown>);
		if (!life) {
			continue;
		}

		const existing = lives.get(life.id);

		// A richer live frame stream wins over the desk row for events;
		// a closed desk row always wins over an open lifecycle (the row
		// carries the settled outcome).
		if (existing && existing.status === "closed" && life.status !== "closed") {
			continue;
		}

		const marks = [
			...(existing?.marks ?? []),
			...((life as { marks?: LifeMark[] }).marks ?? []),
		].slice(-400);

		const merged: Lifecycle = {
			...life,
			policy_epoch: life.policy_epoch ?? existing?.policy_epoch,
			marks_a: life.marks_a ?? existing?.marks_a,
			marks_b: life.marks_b ?? existing?.marks_b,
			marks_c: life.marks_c ?? existing?.marks_c,
			path: life.path ?? existing?.path,
			path_found: life.path_found ?? existing?.path_found,
			stop_state: life.stop_state ?? existing?.stop_state ?? null,
			policy: life.policy ?? existing?.policy ?? null,
			peak_pct: life.peak_pct ?? existing?.peak_pct ?? null,
			realized_pct: life.realized_pct ?? existing?.realized_pct ?? null,
			bars_held: life.bars_held ?? existing?.bars_held ?? null,
			pnl_trace: life.pnl_trace ?? existing?.pnl_trace ?? null,
			exit_trigger: life.exit_trigger ?? existing?.exit_trigger ?? null,
			official_exit_pct:
				life.official_exit_pct ?? existing?.official_exit_pct ?? null,
			events: existing?.events?.length ? existing.events : life.events,
			outcome: life.outcome ?? existing?.outcome ?? life.outcome ?? null,
			mark: life.mark ?? existing?.mark ?? null,
			marks: marks.length > 0 ? marks : existing?.marks,
		};

		if (storeLife(merged)) {
			changed = true;
		}
	}

	if (changed) {
		publish();
	}
};

export const ingestDeskSnapshot = () => {
	ingestDeskClosedLots(positionRows(appSignals.positions.state));
};

const deskRowFromClosedLife = (
	life: Lifecycle,
): Record<string, unknown> | null => {
	if (life.status !== "closed" || !life.outcome) {
		return null;
	}

	const outcome = life.outcome;
	const book = life.book ?? "paper";
	return {
		id: life.id,
		symbol: life.symbol,
		book,
		virtual: book === "virtual",
		path_found: life.path_found === true,
		opened_at: life.opened_at,
		closed_at: life.closed_at,
		pnl: outcome.pnl,
		pnl_pct: outcome.pnl_pct,
		hold_ns: outcome.hold_ns,
		outcome_triggers: outcome.triggers,
		exit_trigger: life.exit_trigger ?? null,
	};
};

/*
deskRowsForStats is the positions snapshot plus closed desk lots the lifecycle
store still holds after Train drops them from the hub frame. Edge and win rate
must use this, not positionRows alone.
*/
export const deskRowsForStats = (): Record<string, unknown>[] => {
	const rows = positionRows(appSignals.positions.state);
	const seen = new Set(
		rows.filter((row) => row.id).map((row) => String(row.id)),
	);

	for (const life of lives.values()) {
		if (!life.desk || life.status !== "closed" || !life.outcome) {
			continue;
		}

		if (seen.has(life.id)) {
			continue;
		}

		const row = deskRowFromClosedLife(life);
		if (row) {
			rows.push(row);
			seen.add(life.id);
		}
	}

	return rows;
};

/* resetLifecycles empties the store (tests). */
export const resetLifecycles = () => {
	lives.clear();
	historyStamp.clear();
	snapshot = { report: summarize([]) };
	for (const listener of listeners) listener();
};

export type ClosedHistoryEntry = {
	stamp: HistoryStamp;
	row: Record<string, unknown>;
};

const historyFrameIsNewerStamp = (
	stamp: HistoryStamp,
	prior: HistoryStamp,
): boolean => {
	if (stamp.epoch !== prior.epoch) return stamp.epoch > prior.epoch;
	if (stamp.tick !== prior.tick) return stamp.tick > prior.tick;
	return stamp.timestamp > prior.timestamp;
};

const ingestHistoryClosedEntry = (entry: ClosedHistoryEntry): boolean => {
	const row = entry.row;
	if (!row || typeof row.id !== "string" || row.id === "") {
		return false;
	}

	const existing = lives.get(row.id);
	if (existing && !existing.history) {
		return false;
	}

	const prior = historyStamp.get(row.id);
	if (prior && !historyFrameIsNewerStamp(entry.stamp, prior)) {
		return false;
	}

	historyStamp.set(row.id, entry.stamp);

	const life = lifecycleFromDeskClosed(row);
	if (!life) {
		return false;
	}

	life.history = true;
	life.desk = true;

	const merged: Lifecycle = {
		...life,
		policy_epoch: life.policy_epoch ?? existing?.policy_epoch,
		marks_a: life.marks_a ?? existing?.marks_a,
		marks_b: life.marks_b ?? existing?.marks_b,
		marks_c: life.marks_c ?? existing?.marks_c,
		path: life.path ?? existing?.path,
		path_found: life.path_found ?? existing?.path_found,
		stop_state: life.stop_state ?? existing?.stop_state ?? null,
		policy: life.policy ?? existing?.policy ?? null,
		peak_pct: life.peak_pct ?? existing?.peak_pct ?? null,
		realized_pct: life.realized_pct ?? existing?.realized_pct ?? null,
		bars_held: life.bars_held ?? existing?.bars_held ?? null,
		pnl_trace: life.pnl_trace ?? existing?.pnl_trace ?? null,
		exit_trigger: life.exit_trigger ?? existing?.exit_trigger ?? null,
		official_exit_pct:
			life.official_exit_pct ?? existing?.official_exit_pct ?? null,
		events: existing?.events?.length ? existing.events : life.events,
		outcome: life.outcome ?? existing?.outcome ?? null,
		mark: life.mark ?? existing?.mark ?? null,
		marks: existing?.marks?.length ? existing.marks : life.marks,
	};

	return storeLife(merged);
};

/*
ingestHistoryClosedBatch folds many stored closed lots with one publish.
Hub ListPrefix sends JSON {"closed":{entries}} envelopes.
*/
export const ingestHistoryClosedBatch = (entries: ClosedHistoryEntry[]) => {
	let changed = false;

	for (const entry of entries) {
		if (ingestHistoryClosedEntry(entry)) {
			changed = true;
		}
	}

	if (changed) {
		publish();
	}
};

let closedHistoryRequested = false;

/*
requestClosedPositionHistory asks the hub to list every closed lot in S3
across virtual, paper, and real. ListPrefix returns one frame per book prefix.
*/
export const requestClosedPositionHistory = () => {
	if (closedHistoryRequested) return;
	closedHistoryRequested = true;

	trySendCommand({ type: "s3", prefix: "paper/closed" });
	trySendCommand({ type: "s3", prefix: "virtual/closed" });
	trySendCommand({ type: "s3", prefix: "real/closed" });
};

/** Tests reset the one-shot S3 history request guard. */
export const resetClosedHistoryRequest = () => {
	closedHistoryRequested = false;
};

const subscribe = (listener: () => void) => {
	listeners.add(listener);
	return () => {
		listeners.delete(listener);
	};
};

/*
ensureDeskLifecycleFeed keeps the lifecycle store fed from the desk's lot
frames on a timer, so the Position Life-Cycle tab shows open lots and
settled closes even when no rich frame stream replayed them. The longtail
recorder ingests the same rows; one interval is enough for both.
*/
let lifecycleFeedStarted = false;

export const ensureDeskLifecycleFeed = () => {
	if (lifecycleFeedStarted) return;
	lifecycleFeedStarted = true;

	const tick = () => {
		ingestDeskClosedLots(positionRows(appSignals.positions.state));
	};

	tick();
	window.setInterval(tick, 2_000);
};

export const useLifecycles = (): Snapshot => {
	// The desk feed is an interval side effect: start it once from an effect,
	// never during render (and never when there is no window to own it).
	useEffect(() => {
		if (typeof window === "undefined") return;
		ensureDeskLifecycleFeed();
		requestClosedPositionHistory();
	}, []);

	return useSyncExternalStore(
		subscribe,
		() => snapshot,
		() => snapshot,
	);
};

/*
selectBook keeps one book. A life with no book is a paper trade.
*/
export const selectBook = (
	report: LifecycleReport,
	book: "paper" | "virtual" | "real",
): LifecycleReport =>
	summarize(
		report.lifecycles.filter((life) => (life.book ?? "paper") === book),
	);

/* NOT_AVAILABLE is what a real-exchange figure reads under paper trading. */
export const NOT_AVAILABLE = "not available";

export const signedMoney = (value: number) =>
	`${value >= 0 ? "+" : ""}${value.toFixed(2)}`;

export const ratePercent = (value: number | null | undefined) =>
	value === null || value === undefined ? "—" : `${(100 * value).toFixed(1)}%`;

export const seconds = (nanoseconds: number) => {
	const s = nanoseconds / 1e9;
	if (s < 120) return `${s.toFixed(1)}s`;
	if (s < 7200) return `${(s / 60).toFixed(1)}m`;
	return `${(s / 3600).toFixed(2)}h`;
};
