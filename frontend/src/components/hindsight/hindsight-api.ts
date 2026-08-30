import type {
	HindsightCapture,
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
): Promise<HindsightCapture[]> => {
	const response = await fetch(
		`${hindsightBaseUrl()}/hindsight/captures?run=${encodeURIComponent(run)}`,
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
