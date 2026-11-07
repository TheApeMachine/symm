/*
Human words and tones for DynamicStoploss so a held lot is never a black box.
State comes from broker.Stoploss on each position frame.
*/

import type { DynamicStoplossState } from "#/collections/types";

export type Tone = "good" | "warn" | "bad" | "info" | "neutral";

export const toneColor = (tone: Tone): string =>
	tone === "good"
		? "var(--up)"
		: tone === "warn"
			? "var(--warn)"
			: tone === "bad"
				? "var(--down)"
				: tone === "info"
					? "var(--info)"
					: "var(--f3)";

export type MeterTone = "success" | "warning" | "error" | "info" | "disabled";

export const meterTone = (tone: Tone): MeterTone =>
	tone === "good"
		? "success"
		: tone === "warn"
			? "warning"
			: tone === "bad"
				? "error"
				: tone === "info"
					? "info"
					: "disabled";

export const proximityPct = (
	ratio: number | null | undefined,
): number | null => {
	if (ratio === null || ratio === undefined || !Number.isFinite(ratio)) {
		return null;
	}
	return Math.max(0, Math.min(100, ratio * 100));
};

export const stateTone = (
	state: DynamicStoplossState | null | undefined,
): Tone => {
	switch (state) {
		case "Healthy":
			return "good";
		case "Armed":
			return "info";
		case "Triggered":
			return "bad";
		default:
			return "neutral";
	}
};

const fmtPct = (value: number | null): string =>
	value === null || !Number.isFinite(value) ? "—" : `${value.toFixed(1)}%`;

export const heldVerdict = (args: {
	state: DynamicStoplossState | null | undefined;
	current: number | null;
	peak: number | null;
	floor: number | null;
	friction: number | null;
	has_floor?: boolean;
	proximity?: number | null;
	exit_reason?: string | null;
	room_pct?: number | null;
}): { text: string; tone: Tone } => {
	if (args.state === "Triggered") {
		return {
			text:
				args.exit_reason && args.exit_reason !== ""
					? args.exit_reason
					: "Stop triggered — desk should exit on this mark",
			tone: "bad",
		};
	}

	if (args.state === "Armed") {
		const room =
			args.room_pct !== null &&
			args.room_pct !== undefined &&
			Number.isFinite(args.room_pct)
				? `${Math.abs(args.room_pct).toFixed(1)}pp room`
				: null;
		return {
			text: room
				? `Armed — ${room} before floor at ${fmtPct(args.floor)}`
				: `Armed — floor at ${fmtPct(args.floor)}`,
			tone:
				typeof args.proximity === "number" &&
				Number.isFinite(args.proximity) &&
				args.proximity >= 0.7
					? "warn"
					: "info",
		};
	}

	const current = args.current;
	if (current === null || !Number.isFinite(current)) {
		return {
			text: "Waiting for the desk book to price the lot",
			tone: "neutral",
		};
	}

	const peakKnown = args.peak !== null && Number.isFinite(args.peak);
	if (peakKnown && args.peak !== null && args.peak > (args.friction ?? 0)) {
		return {
			text: `In profit (peak ${args.peak.toFixed(1)}%) — waiting to arm`,
			tone: "good",
		};
	}

	return {
		text: `Holding near entry (${current.toFixed(2)}%) — stop healthy`,
		tone: "good",
	};
};

export type StopCheck = {
	id: "floor" | "peak" | "state";
	name: string;
	role: "defence" | "harvest" | "context";
	word: StopWord;
	percent: number | null;
	tone: Tone;
};

export type StopWord = { label: string; short: string; title: string };

const floorTone = (proximity: number | null | undefined): Tone => {
	if (
		proximity === null ||
		proximity === undefined ||
		!Number.isFinite(proximity)
	) {
		return "neutral";
	}
	if (proximity >= 1) return "bad";
	if (proximity >= 0.7) return "warn";
	if (proximity >= 0.35) return "info";
	return "good";
};

export const buildStopChecks = (args: {
	state: DynamicStoplossState | null | undefined;
	proximity?: number | null;
	has_floor?: boolean;
	room_pct?: number | null;
	current?: number | null;
	peak?: number | null;
}): StopCheck[] => {
	const prox = proximityPct(args.proximity);

	const floorShort =
		args.has_floor && args.room_pct !== null && args.room_pct !== undefined
			? `${Math.max(0, args.room_pct).toFixed(1)}pp room`
			: args.has_floor
				? prox !== null
					? `${Math.round(prox)}% to floor`
					: "armed"
				: "no floor";

	return [
		{
			id: "floor",
			name: "Floor",
			role: "harvest",
			word: {
				label: "Floor proximity",
				short: floorShort,
				title: args.has_floor
					? `Distance from peak toward the give-back floor. ${prox !== null ? `${Math.round(prox)}% of the peak→floor span consumed` : "Floor is set"}${args.current !== null && args.current !== undefined ? `, mark ${fmtPct(args.current)}` : ""}.`
					: "Floor has not armed yet — trail engages once mark clears the arm threshold.",
			},
			percent: args.has_floor ? prox : null,
			tone: args.has_floor ? floorTone(args.proximity) : "neutral",
		},
		{
			id: "peak",
			name: "Peak",
			role: "context",
			word: {
				label: "Peak mark",
				short:
					args.peak !== null && args.peak !== undefined
						? fmtPct(args.peak)
						: "—",
				title:
					args.peak !== null && args.peak !== undefined
						? `Best fee-net mark this hold: ${fmtPct(args.peak)}.`
						: "No peak printed yet.",
			},
			percent: null,
			tone: "info",
		},
		{
			id: "state",
			name: "State",
			role: "defence",
			word: {
				label: "Stop state",
				short: args.state ?? "—",
				title: args.state
					? `Live stop state: ${args.state}.`
					: "No stop telemetry on this lot.",
			},
			percent: null,
			tone: stateTone(args.state),
		},
	];
};

export const stopRule = (
	label: string,
	short: string,
	title: string,
): StopWord => ({ label, short, title });
