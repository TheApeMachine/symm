import type {
	HindsightCapture,
	HindsightMetricMap,
	HindsightTimeline,
	HindsightTimelineQuery,
	HindsightEnvelope,
	HindsightGap,
	HindsightLifecycleEvent,
	HindsightRun,
	HindsightState,
} from "./hindsight-types";

/*
hindsightBaseUrl mirrors the other REST readers: derive the hub origin from the
websocket URL (env override with a localhost default), then hit the read-only
/hindsight/* endpoints the hub serves from the persisted store.
*/
const hindsightBaseUrl = () => {
	if (import.meta.env.VITE_SYMM_WS_URL) {
		return import.meta.env.VITE_SYMM_WS_URL.replace(/^ws/, "http").replace(/\/ws$/, "");
	}

	const protocol = window.location.protocol === "https:" ? "https:" : "http:";
	const host =
		!window.location.hostname || window.location.hostname === "localhost"
			? "127.0.0.1"
			: window.location.hostname;

	return `${protocol}//${host}:8765`;
};

export const fetchHindsightRuns = async (): Promise<HindsightRun[]> => {
	const response = await fetch(`${hindsightBaseUrl()}/hindsight/runs`);

	if (!response.ok) {
		return [];
	}

	return (await response.json()) as HindsightRun[];
};

export const fetchHindsightCaptures = async (
	run: string,
	after = 0,
): Promise<HindsightCapture[]> => {
	const response = await fetch(
		`${hindsightBaseUrl()}/hindsight/captures?run=${encodeURIComponent(run)}&after=${after}`,
	);

	if (!response.ok) {
		return [];
	}

	return (await response.json()) as HindsightCapture[];
};

export const fetchHindsightStates = async (
	run: string,
): Promise<HindsightState[]> => {
	const response = await fetch(
		`${hindsightBaseUrl()}/hindsight/states?run=${encodeURIComponent(run)}`,
	);

	if (!response.ok) {
		return [];
	}

	return (await response.json()) as HindsightState[];
};

export const fetchHindsightState = async (
	run: string,
	sequence: number,
	ordinal: number,
): Promise<HindsightState | null> => {
	const response = await fetch(
		`${hindsightBaseUrl()}/hindsight/state?run=${encodeURIComponent(run)}&seq=${sequence}&ordinal=${ordinal}`,
	);

	if (!response.ok) {
		return null;
	}

	const state = (await response.json()) as HindsightState;

	return state.payload ? state : null;
};

export const fetchHindsightEnvelope = async (
	run: string,
	sequence: number,
): Promise<HindsightEnvelope | null> => {
	const response = await fetch(
		`${hindsightBaseUrl()}/hindsight/envelope?run=${encodeURIComponent(run)}&seq=${sequence}`,
	);

	if (!response.ok) {
		return null;
	}

	return (await response.json()) as HindsightEnvelope;
};

export const fetchHindsightGaps = async (
	run: string,
): Promise<HindsightGap[]> => {
	const response = await fetch(
		`${hindsightBaseUrl()}/hindsight/gaps?run=${encodeURIComponent(run)}`,
	);

	if (!response.ok) {
		return [];
	}

	return (await response.json()) as HindsightGap[];
};

export const fetchHindsightLifecycle = async (
	run: string,
): Promise<HindsightLifecycleEvent[]> => {
	const response = await fetch(
		`${hindsightBaseUrl()}/hindsight/lifecycle?run=${encodeURIComponent(run)}`,
	);

	if (!response.ok) {
		return [];
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
		`${hindsightBaseUrl()}/hindsight/timeline?${params.toString()}`,
	);

	if (!response.ok) {
		return null;
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
		const response = await fetch(`${hindsightBaseUrl()}/hindsight/metric-map`);

		if (!response.ok) {
			return null;
		}

		return (await response.json()) as HindsightMetricMap;
	};
