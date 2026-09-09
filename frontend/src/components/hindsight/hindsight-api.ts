import type {
	HindsightCapture,
	HindsightEnvelope,
	HindsightGap,
	HindsightLifecycleEvent,
	HindsightMetricMap,
	HindsightResident,
	HindsightRun,
	HindsightState,
	HindsightTimeline,
	HindsightTimelineQuery,
} from "./hindsight-types";
import { hubBaseUrl } from "#/lib/hub";


export const fetchHindsightRuns = async (): Promise<HindsightRun[]> => {
	const response = await fetch(`${hubBaseUrl()}/hindsight/runs`);

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	const runs: HindsightRun[] = await response.json();

	if (!Array.isArray(runs))
		throw new Error("Hindsight returned invalid run metadata");

	for (const run of runs) {
		if (
			typeof run?.id !== "string" ||
			run.id === "" ||
			typeof run.startedAt !== "string"
		) {
			throw new Error("Hindsight returned invalid run metadata");
		}
		// The date formatter must never receive an unparseable timestamp.
		new Date(run.startedAt).toISOString();
	}
	return runs;
};

export const fetchHindsightCaptures = async (
	run: string,
	after = 0,
): Promise<HindsightCapture[]> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/captures?run=${encodeURIComponent(run)}&after=${after}`,
	);

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	return (await response.json()) as HindsightCapture[];
};

export const fetchHindsightStates = async (
	run: string,
): Promise<HindsightState[]> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/states?run=${encodeURIComponent(run)}`,
	);

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	return (await response.json()) as HindsightState[];
};

export const fetchHindsightState = async (
	run: string,
	sequence: number,
	ordinal: number,
): Promise<HindsightState | null> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/state?run=${encodeURIComponent(run)}&seq=${sequence}&ordinal=${ordinal}`,
	);

	if (response.status === 404) return null;

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	const state = (await response.json()) as HindsightState;

	return state.payload ? state : null;
};

export const fetchHindsightEnvelope = async (
	run: string,
	sequence: number,
): Promise<HindsightEnvelope | null> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/envelope?run=${encodeURIComponent(run)}&seq=${sequence}`,
	);

	if (response.status === 404) return null;

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	return (await response.json()) as HindsightEnvelope;
};

export const fetchHindsightGaps = async (
	run: string,
): Promise<HindsightGap[]> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/gaps?run=${encodeURIComponent(run)}`,
	);

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	return (await response.json()) as HindsightGap[];
};

export const fetchHindsightLifecycle = async (
	run: string,
): Promise<HindsightLifecycleEvent[]> => {
	const response = await fetch(
		`${hubBaseUrl()}/hindsight/lifecycle?run=${encodeURIComponent(run)}`,
	);

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	return (await response.json()) as HindsightLifecycleEvent[];
};

/*
fetchHindsightTimeline reads the Episode projection of one Run: the declared
coordinate bucketed along the chosen axis, the episodes a declared selector
found on it, the transport spans, and the instrument index. Every parameter of
the selector is a query argument, so what the view calls interesting is always
stated in the request that produced it.
*/
export const fetchHindsightTimeline = async (
	query: HindsightTimelineQuery,
): Promise<HindsightTimeline | null> => {
	const params = new URLSearchParams({ run: query.run });

	if (query.symbol) params.set("symbol", query.symbol);
	if (query.coordinate) params.set("coordinate", query.coordinate);
	if (query.axis) params.set("axis", query.axis);
	if (query.buckets) params.set("buckets", String(query.buckets));
	if (query.from) params.set("from", String(query.from));
	if (query.to) params.set("to", String(query.to));
	if (query.symbols) params.set("symbols", "1");

	const response = await fetch(
		`${hubBaseUrl()}/hindsight/timeline?${params.toString()}`,
	);

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	return (await response.json()) as HindsightTimeline;
};

/*
fetchHindsightMetricMap reads the declared semantics of every production
metric. It is the same answer for every run and every capture, so it is fetched
once per session rather than per inspected frame.
*/
export const fetchHindsightMetricMap =
	async (): Promise<HindsightMetricMap | null> => {
		const response = await fetch(`${hubBaseUrl()}/hindsight/metric-map`);

		if (!response.ok) {
			throw new Error(
				`Hindsight request failed (${response.status}): ${await response.text()}`,
			);
		}

		return (await response.json()) as HindsightMetricMap;
	};

/*
fetchHindsightResident reads what the running system actually held at one exact
capture coordinate, rather than what that one envelope carried. The full
coordinate is (sequence, ordinal): one raw capture can produce several
envelopes, and an ordinal outside the target's causal past is future state and
never consulted. The budget bounds how far back the causal walk may reach; the
answer reports how far it actually went.
*/
export const fetchHindsightResident = async (
	run: string,
	symbol: string,
	sequence: number,
	ordinal: number,
	budget = 96,
): Promise<HindsightResident | null> => {
	const params = new URLSearchParams({
		run,
		symbol,
		seq: String(sequence),
		ordinal: String(ordinal),
		budget: String(budget),
	});

	const response = await fetch(
		`${hubBaseUrl()}/hindsight/resident?${params.toString()}`,
	);

	if (!response.ok) {
		throw new Error(
			`Hindsight request failed (${response.status}): ${await response.text()}`,
		);
	}

	return (await response.json()) as HindsightResident;
};
