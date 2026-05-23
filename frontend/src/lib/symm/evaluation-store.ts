import type { DecisionTraceEvent, EnginePulseEvent } from "#/lib/symm/events";

export type SignalReading = {
	source: string;
	regime: string;
	reason: string;
	confidence: number;
	ts: string;
};

export type SymbolEvaluation = {
	symbol: string;
	combined: number;
	support: number;
	regime: string;
	reason: string;
	allow: boolean;
	why: string;
	signals: Record<string, SignalReading>;
	updatedAt: string;
};

export type EvaluationState = {
	seq: number;
	phase: string;
	line: number;
	median: number;
	mad: number;
	bySymbol: Map<string, SymbolEvaluation>;
};

export const emptyEvaluationState = (): EvaluationState => ({
	seq: 0,
	phase: "warming",
	line: 0,
	median: 0,
	mad: 0,
	bySymbol: new Map(),
});

function upsertReading(
	signals: Record<string, SignalReading>,
	row: EnginePulseEvent["signals"][number],
	ts: string,
): Record<string, SignalReading> {
	return {
		...signals,
		[row.source]: {
			source: row.source,
			regime: row.regime,
			reason: row.reason,
			confidence: row.score,
			ts,
		},
	};
}

export function mergeEnginePulse(
	state: EvaluationState,
	pulse: EnginePulseEvent,
): EvaluationState {
	const ts = pulse.ts ?? new Date().toISOString();
	const bySymbol = new Map(state.bySymbol);

	for (const row of pulse.signals ?? []) {
		const existing = bySymbol.get(row.symbol);
		const signals = upsertReading(existing?.signals ?? {}, row, ts);

		bySymbol.set(row.symbol, {
			symbol: row.symbol,
			combined: existing?.combined ?? row.score,
			support: existing?.support ?? 1,
			regime: row.regime,
			reason: row.reason,
			allow: existing?.allow ?? false,
			why: existing?.why ?? "signal_only",
			signals,
			updatedAt: ts,
		});
	}

	return {
		...state,
		seq: pulse.seq,
		phase: pulse.phase,
		bySymbol,
	};
}

export function mergeDecisionTrace(
	state: EvaluationState,
	trace: DecisionTraceEvent,
): EvaluationState {
	const ts = trace.ts;
	const bySymbol = new Map(state.bySymbol);

	for (const row of trace.evaluations ?? []) {
		const signalsList = row.signals ?? [];
		const signals: Record<string, SignalReading> = {};

		for (const reading of signalsList) {
			const source = String(reading.source ?? "");
			if (!source) {
				continue;
			}

			signals[source] = {
				source,
				regime: String(reading.regime ?? ""),
				reason: String(reading.reason ?? ""),
				confidence: Number(reading.confidence ?? 0),
				ts,
			};
		}

		bySymbol.set(row.symbol, {
			symbol: row.symbol,
			combined: row.combined,
			support: row.support,
			regime: row.regime,
			reason: row.reason,
			allow: row.allow,
			why: row.why,
			signals,
			updatedAt: ts,
		});
	}

	for (const row of trace.decisions ?? []) {
		if (!row.source) {
			continue;
		}

		const existing = bySymbol.get(row.symbol);
		const signals = upsertReading(
			existing?.signals ?? {},
			{
				symbol: row.symbol,
				source: row.source,
				regime: row.regime,
				reason: row.reason,
				score: row.confidence,
				type: row.regime,
			},
			ts,
		);

		bySymbol.set(row.symbol, {
			symbol: row.symbol,
			combined: existing?.combined ?? row.confidence,
			support: existing?.support ?? 1,
			regime: existing?.regime ?? row.regime,
			reason: existing?.reason ?? row.reason,
			allow: row.allow,
			why: row.why,
			signals,
			updatedAt: ts,
		});
	}

	return {
		...state,
		line: trace.line,
		median: trace.median,
		mad: trace.mad,
		bySymbol,
	};
}

export function rankedEvaluations(state: EvaluationState): SymbolEvaluation[] {
	return [...state.bySymbol.values()].sort((left, right) => {
		if (left.combined !== right.combined) {
			return right.combined - left.combined;
		}

		return left.symbol.localeCompare(right.symbol);
	});
}
