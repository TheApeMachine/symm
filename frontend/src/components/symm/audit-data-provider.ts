import type { AuditEvent } from "#/lib/symm/events";
import { isAuditEvent } from "#/lib/symm/events";

export type AuditRow = {
	key: string;
	seq: number;
	event: string;
	ts: string;
	symbol?: string;
	source?: string;
	reason?: string;
	summary: string;
};

type Listener = () => void;

const MAX_ROWS = 120;
const SUMMARY_KEYS = [
	"edge",
	"confidence",
	"predicted_return",
	"actual_return",
	"error",
	"urgency",
	"success",
	"why",
	"slot_eur",
] as const;

const formatValue = (value: unknown): string => {
	if (typeof value === "number") {
		return Number.isInteger(value) ? value.toString() : value.toPrecision(4);
	}

	if (typeof value === "boolean") {
		return value ? "true" : "false";
	}

	if (typeof value === "string") {
		return value;
	}

	return "";
};

const summarize = (event: AuditEvent): string => {
	const parts: string[] = [];

	for (const key of SUMMARY_KEYS) {
		const value = event[key];

		if (value === undefined || value === null) {
			continue;
		}

		const formatted = formatValue(value);

		if (formatted === "") {
			continue;
		}

		parts.push(`${key}=${formatted}`);
	}

	return parts.join(" · ");
};

class AuditDataProviderImpl {
	private rows: AuditRow[] = [];
	private listeners = new Set<Listener>();

	subscribe(listener: Listener) {
		this.listeners.add(listener);

		return () => {
			this.listeners.delete(listener);
		};
	}

	snapshot(): readonly AuditRow[] {
		return this.rows;
	}

	private notify() {
		for (const listener of this.listeners) {
			listener();
		}
	}

	ingest(raw: unknown) {
		if (!isAuditEvent(raw)) {
			return;
		}

		this.rows = [
			{
				key: `${raw.seq}:${raw.audit_event}`,
				seq: raw.seq,
				event: raw.audit_event,
				ts: raw.ts,
				symbol: raw.symbol,
				source: raw.source,
				reason: raw.reason,
				summary: summarize(raw),
			},
			...this.rows,
		].slice(0, MAX_ROWS);
		this.notify();
	}

	reset() {
		this.rows = [];
		this.notify();
	}
}

const shared = new AuditDataProviderImpl();

export const AuditDataProvider = {
	subscribe: (listener: Listener) => shared.subscribe(listener),
	snapshot: () => shared.snapshot(),
	ingest: (raw: unknown) => shared.ingest(raw),
	reset: () => shared.reset(),
};
