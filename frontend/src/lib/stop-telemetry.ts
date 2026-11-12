/*
Human words and tones for a held lot's Stoploss (broker.Stoploss on the wire).
*/

import {
	STOPLOSS_ARMED,
	STOPLOSS_HEALTHY,
	STOPLOSS_TRIGGERED,
	type Stoploss,
} from "#/collections/types";
import { pctMark } from "#/lib/format";

export type Tone = "good" | "warn" | "bad" | "info" | "neutral";

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

export const stopRoom = (stop: Stoploss) =>
	stop.CurrentPnLPct - stop.FloorPnLPct;

export const stopProximity = (stop: Stoploss) => {
	const span = Math.max(1e-9, stop.PeakPnLPct - stop.FloorPnLPct);
	return Math.max(0, Math.min(1, 1 - stopRoom(stop) / span));
};

export const stopArmed = (stop: Stoploss) =>
	stop.State === STOPLOSS_ARMED || stop.State === STOPLOSS_TRIGGERED;

export const stateTone = (state: number | null | undefined): Tone => {
	switch (state) {
		case STOPLOSS_HEALTHY:
			return "good";
		case STOPLOSS_ARMED:
			return "info";
		case STOPLOSS_TRIGGERED:
			return "bad";
		default:
			return "neutral";
	}
};

export const heldVerdict = (
	stop: Stoploss | null | undefined,
): { text: string; tone: Tone } => {
	if (!stop) {
		return {
			text: "Waiting for the desk book to price the lot",
			tone: "neutral",
		};
	}

	if (stop.State === STOPLOSS_TRIGGERED) {
		return {
			text: "Floor hit — desk should exit on this mark",
			tone: "bad",
		};
	}

	if (stop.State === STOPLOSS_ARMED) {
		const room = stopRoom(stop);
		const proximity = stopProximity(stop);
		return {
			text: `Armed — ${Math.abs(room).toFixed(1)}pp before floor at ${pctMark(stop.FloorPnLPct, 1)}`,
			tone: proximity >= 0.7 ? "warn" : "info",
		};
	}

	if (stop.PeakPnLPct > Math.abs(stop.InitialPnLPct)) {
		return {
			text: `In profit (peak ${stop.PeakPnLPct.toFixed(1)}%) — waiting to arm`,
			tone: "good",
		};
	}

	return {
		text: `Holding near entry (${stop.CurrentPnLPct.toFixed(2)}%) — healthy`,
		tone: "good",
	};
};

export type StopCheck = {
	id: "floor" | "peak" | "state";
	name: string;
	role: "defence" | "harvest" | "context";
	word: { label: string; short: string; title: string };
	percent: number | null;
	tone: Tone;
};

export const buildStopChecks = (
	stop: Stoploss | null | undefined,
): StopCheck[] => {
	if (!stop) return [];

	const proximity = stopProximity(stop);
	const proxPct = Math.max(0, Math.min(100, proximity * 100));
	const room = stopRoom(stop);
	const armed = stopArmed(stop);

	return [
		{
			id: "floor",
			name: "Floor",
			role: "defence",
			word: {
				label: armed ? `${room.toFixed(1)}pp to floor` : "not armed",
				short: armed ? `${proxPct.toFixed(0)}%` : "—",
				title: `Floor ${stop.FloorPnLPct.toFixed(2)}%`,
			},
			percent: armed ? proxPct : null,
			tone:
				proximity >= 1
					? "bad"
					: proximity >= 0.7
						? "warn"
						: proximity >= 0.35
							? "info"
							: "good",
		},
		{
			id: "peak",
			name: "Peak",
			role: "harvest",
			word: {
				label: `${stop.PeakPnLPct.toFixed(2)}%`,
				short: `${stop.PeakPnLPct.toFixed(1)}%`,
				title: "Peak PnL%",
			},
			percent: null,
			tone: "info",
		},
		{
			id: "state",
			name: "State",
			role: "context",
			word: {
				label:
					stop.State === STOPLOSS_TRIGGERED
						? "Triggered"
						: stop.State === STOPLOSS_ARMED
							? "Armed"
							: "Healthy",
				short:
					stop.State === STOPLOSS_TRIGGERED
						? "HIT"
						: stop.State === STOPLOSS_ARMED
							? "ARM"
							: "OK",
				title: "Stoploss state",
			},
			percent: null,
			tone: stateTone(stop.State),
		},
	];
};
