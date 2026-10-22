import { positionRows, signals as appSignals } from "#/collections/app";
import {
	lifecycleFromDeskClosed,
	lifecycleFromDeskOpen,
} from "#/lib/desk-closed-lifecycle";
import { useEffect, useSyncExternalStore } from "react";

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
	cost: number;
	proceeds: number;
	fees: number;
	realized: number;
	net_return: number;
	hold_ns: number;
	expected_hold_ns: number | null;
	triggers: string[] | null;
};

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
	/** Bid marks sampled while the lot was open (for hold charts). */
	marks?: LifeMark[];
	outcome?: LifeOutcome | null;
	book?: string;
	/**
	Tape ticks the closed round trip covered, carried on the desk position's
	own run epoch: A some prints before the entry, B the entry, C the exit.
	The Position Life-Cycle reads these as training-sample markers.
	*/
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
};

type Snapshot = { report: LifecycleReport };

const LIFECYCLE_LIMIT = 200;
const lives = new Map<string, Lifecycle>();
const listeners = new Set<() => void>();

const summarize = (all: Lifecycle[]): LifecycleReport => {
	const closed = all.filter((life) => life.status === "closed" && life.outcome);
	const returns = closed.map(
		(life) => (life.outcome as LifeOutcome).net_return,
	);
	const wins = closed.filter(
		(life) => (life.outcome as LifeOutcome).realized > 0,
	);

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
			pnl: closed.reduce(
				(sum, life) => sum + (life.outcome as LifeOutcome).realized,
				0,
			),
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

const remember = (life: Lifecycle) => {
	lives.delete(life.id);
	lives.set(life.id, life);

	while (lives.size > LIFECYCLE_LIMIT) {
		let evict: string | undefined;
		for (const [id, life] of lives) {
			if (life.status === "closed" || life.status === "abandoned") {
				evict = id;
				break;
			}
		}
		if (evict === undefined) break;
		lives.delete(evict);
	}

	publish();
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
		marks: marks.length > 0 ? marks : life.marks,
		events: frame.event ? [...(life.events ?? []), frame.event] : life.events,
	};

	remember(next);
};

/*
ingestDeskClosedLots merges persisted desk round trips into the lifecycle
report when the live position frame stream did not replay them.
*/
export const ingestDeskClosedLots = (rows: Record<string, unknown>[]) => {
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

		remember({
			...life,
			events: existing?.events?.length ? existing.events : life.events,
			outcome: life.outcome ?? existing?.outcome ?? life.outcome ?? null,
			mark: life.mark ?? existing?.mark ?? null,
			marks: marks.length > 0 ? marks : existing?.marks,
		});
	}
};

/* resetLifecycles empties the store (tests). */
export const resetLifecycles = () => {
	lives.clear();
	snapshot = { report: summarize([]) };
	for (const listener of listeners) listener();
};

const subscribe = (listener: () => void) => {
	listeners.add(listener);
	return () => {
		listeners.delete(listener);
	};
};

/*
ensureDeskLifecycleFeed keeps the lifecycle store fed from the desk's merged
position frames on a timer, so the Position Life-Cycle tab shows open lots and
settled closes even when no rich frame stream replayed them. The longtail
recorder ingests the same rows; one interval is enough for both.
*/
let lifecycleFeedStarted = false;

export const ensureDeskLifecycleFeed = () => {
	if (lifecycleFeedStarted) return;
	lifecycleFeedStarted = true;

	const tick = () => {
		const rows = positionRows(appSignals.positions, "all") as Record<
			string,
			unknown
		>[];
		ingestDeskClosedLots(rows);
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
	book: "paper" | "virtual",
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
