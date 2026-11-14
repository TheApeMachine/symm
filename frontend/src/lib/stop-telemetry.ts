/*
Human words and tones for a held position's Stoploss (broker.Stoploss on the wire).
Exit arms in fee-net profit, then confirms near the crest (Peak × nearPeakFrac)
with kinematic exhaustion — not a trailing floor giveback.
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

/** Matches system.Cfg.Selection.StoplossNearPeakFrac default. */
export const NEAR_PEAK_FRAC = 0.98;

/** Matches system.Cfg.Selection.StoplossConfirmMarks default. */
export const CONFIRM_MARKS = 3;

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

/** Lower edge of the crest corridor — exit confirms only while Current ≥ this. */
export const nearPeakFloor = (stop: Stoploss) => stop.PeakPnLPct * NEAR_PEAK_FRAC;

/** pp still above the crest floor (negative ⇒ slipped out of the harvest zone). */
export const stopRoom = (stop: Stoploss) =>
	stop.CurrentPnLPct - nearPeakFloor(stop);

/** 0 at peak → 1 at/below crest floor. */
export const stopProximity = (stop: Stoploss) => {
	const span = Math.max(1e-9, stop.PeakPnLPct - nearPeakFloor(stop));
	return Math.max(
		0,
		Math.min(1, (stop.PeakPnLPct - stop.CurrentPnLPct) / span),
	);
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
			text: "Waiting for the desk book to price the position",
			tone: "neutral",
		};
	}

	if (stop.State === STOPLOSS_TRIGGERED) {
		return {
			text: "Crest confirmed — desk should exit on this mark",
			tone: "bad",
		};
	}

	if (stop.State === STOPLOSS_ARMED) {
		const room = stopRoom(stop);
		const proximity = stopProximity(stop);
		const confirms = stop.Confirms ?? 0;
		const exh = stop.ExhaustScore ?? 0;
		return {
			text: `Armed — crest ${pctMark(stop.PeakPnLPct, 1)}, ${room.toFixed(1)}pp above floor · c${confirms}/${CONFIRM_MARKS} exh${exh.toFixed(2)}`,
			tone: proximity >= 0.7 || confirms >= 2 ? "warn" : "info",
		};
	}

	if (stop.PeakPnLPct > Math.abs(stop.InitialPnLPct)) {
		return {
			text: `In profit (peak ${stop.PeakPnLPct.toFixed(1)}%) — waiting to arm at ${stop.ArmPnlPct.toFixed(1)}%`,
			tone: "good",
		};
	}

	return {
		text: `Holding near entry (${stop.CurrentPnLPct.toFixed(2)}%) — healthy`,
		tone: "good",
	};
};

export type StopCheck = {
	id: "crest" | "peak" | "confirm" | "state";
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
	const confirms = stop.Confirms ?? 0;
	const exh = stop.ExhaustScore ?? 0;
	const confirmPct = Math.max(
		0,
		Math.min(100, (confirms / CONFIRM_MARKS) * 100),
	);

	return [
		{
			id: "crest",
			name: "Crest",
			role: "defence",
			word: {
				label: armed ? `${room.toFixed(1)}pp in crest` : "not armed",
				short: armed ? `${proxPct.toFixed(0)}%` : "—",
				title: `Near-peak floor ${nearPeakFloor(stop).toFixed(2)}%`,
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
			id: "confirm",
			name: "Confirm",
			role: "harvest",
			word: {
				label: armed ? `c${confirms}/${CONFIRM_MARKS} exh${exh.toFixed(2)}` : "—",
				short: armed ? `${confirmPct.toFixed(0)}%` : "—",
				title: "Exit confirmation marks × kinematic exhaust",
			},
			percent: armed ? confirmPct : null,
			tone:
				confirms >= CONFIRM_MARKS
					? "bad"
					: confirms >= 2 || exh >= 0.7
						? "warn"
						: "info",
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
