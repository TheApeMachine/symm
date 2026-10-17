import { useSyncExternalStore } from "react";
import { positionCountAtom } from "#/collections/app";

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
	opened_at: string;
	closed_at?: string | null;
	events: LifeEvent[] | null;
	mark?: LifeMark | null;
	outcome?: LifeOutcome | null;
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
	positionCountAtom.set(snapshot.report.open_count);
	for (const listener of listeners) listener();
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

	const next: Lifecycle = {
		...life,
		status: frame.status,
		closed_at: frame.closed_at ?? life.closed_at,
		outcome: frame.outcome ?? life.outcome,
		mark: frame.mark ?? life.mark,
		events: frame.event ? [...(life.events ?? []), frame.event] : life.events,
	};

	lives.delete(frame.id);
	lives.set(frame.id, next);

	while (lives.size > LIFECYCLE_LIMIT) {
		const oldest = lives.keys().next().value;
		if (oldest === undefined) break;
		lives.delete(oldest);
	}

	publish();
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

export const useLifecycles = (): Snapshot =>
	useSyncExternalStore(
		subscribe,
		() => snapshot,
		() => snapshot,
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
