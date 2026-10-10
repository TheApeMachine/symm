import { useSyncExternalStore } from "react";
import { hubBaseUrl } from "#/lib/hub";

/*
Types of GET /positions/lifecycle (broker.LifecycleReport). Times are RFC 3339
strings; durations are nanoseconds.
*/
export type LifeEvent = {
	at: string;
	venue_at?: string;
	kind: string;
	detail: string;
	fields?: Record<string, unknown>;
};

export type LifeOutcome = {
	venue_cost: number;
	venue_proceeds: number;
	venue_fees: number;
	venue_realized: number;
	shadow_defined: boolean;
	shadow_cost: number;
	shadow_proceeds: number;
	shadow_fees: number;
	shadow_realized: number;
	shadow_short: number;
	shadow_unpriced: number;
	hold_ns: number;
	expected_hold_ns: number;
	triggers: string[] | null;
};

export type Lifecycle = {
	id: string;
	symbol: string;
	status: string;
	opened_at: string;
	closed_at?: string;
	events: LifeEvent[] | null;
	outcome?: LifeOutcome;
};

export type Performance = {
	closed: number;
	shadow_trades: number;
	shadow_wins: number;
	shadow_win_rate: number | null;
	shadow_mean_return: number | null;
	shadow_pnl: number;
	venue_wins: number;
	venue_win_rate: number | null;
	venue_mean_return: number | null;
	venue_pnl: number;
};

export type LifecycleReport = {
	lifecycles: Lifecycle[] | null;
	performance: Performance;
};

/*
One poller serves every subscriber: it runs while anything is subscribed and
keeps the last report it read. A failed read keeps the previous report and
marks the source offline rather than inventing an empty one.
*/
type Snapshot = { report: LifecycleReport | null; online: boolean };

const POLL_MS = 1000;
let snapshot: Snapshot = { report: null, online: false };
const listeners = new Set<() => void>();
let timer: number | undefined;

const publish = (next: Snapshot) => {
	snapshot = next;
	for (const listener of listeners) listener();
};

const poll = async () => {
	try {
		const res = await fetch(`${hubBaseUrl()}/positions/lifecycle`);
		if (!res.ok) {
			publish({ report: snapshot.report, online: false });
			return;
		}
		const report: LifecycleReport = await res.json();
		publish({ report, online: true });
	} catch {
		publish({ report: snapshot.report, online: false });
	}
};

const subscribe = (listener: () => void) => {
	listeners.add(listener);
	if (timer === undefined) {
		void poll();
		timer = window.setInterval(poll, POLL_MS);
	}
	return () => {
		listeners.delete(listener);
		if (listeners.size === 0 && timer !== undefined) {
			clearInterval(timer);
			timer = undefined;
		}
	};
};

export const useLifecycles = (): Snapshot =>
	useSyncExternalStore(subscribe, () => snapshot, () => snapshot);

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
