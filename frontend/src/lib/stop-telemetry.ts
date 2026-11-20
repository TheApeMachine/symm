/*
Human words and tones for a held position's Stoploss (broker.Stoploss on the wire).
Floor / n1 / n2 come from the live stop — not Ascent path memory.
*/

import {
	type MarketRegime,
	STOPLOSS_ARMED,
	STOPLOSS_HEALTHY,
	STOPLOSS_TRIGGERED,
	type Stoploss,
} from "#/collections/types";

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

export const climbAboveArm = (stop: Stoploss) =>
	Math.max(0, stop.PeakPnLPct - stop.ArmPnlPct);

/** Desk trip floor — prefers live TrailingFloor, else typical pullback room. */
export const givebackFloor = (stop: Stoploss) => {
	if (
		typeof stop.TrailingFloor === "number" &&
		Number.isFinite(stop.TrailingFloor)
	) {
		return stop.TrailingFloor;
	}
	const n2 = Math.max(0, stop.TypicalPullback ?? 0);
	const climb = climbAboveArm(stop);
	const room = n2 > 0 ? n2 * 1.35 : climb / 2;
	return stop.PeakPnLPct - room;
};

export const stopRoom = (stop: Stoploss) =>
	stop.CurrentPnLPct - givebackFloor(stop);

/** 0 at peak → 1 at/below trailing floor. */
export const stopProximity = (stop: Stoploss) => {
	const peak = stop.PeakPnLPct;
	const floor = givebackFloor(stop);
	const span = peak - floor;
	if (span <= 1e-9) return 0;
	const drop = Math.max(0, peak - stop.CurrentPnLPct);
	return Math.max(0, Math.min(1, drop / span));
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

export const regimeLabel = (regime?: MarketRegime) => {
	switch (regime) {
		case "CRAWL":
			return "Crawl";
		case "SMOOTH":
			return "Smooth";
		case "EXPLOSIVE":
			return "Explosive";
		case "CHAOS":
			return "Chaos";
		default:
			return regime || "—";
	}
};

export const CONFIRM_MARKS_MAX = 2;

export const confirmsNeeded = (stop?: Stoploss) =>
	stop?.Regime === "EXPLOSIVE" ? 1 : CONFIRM_MARKS_MAX;

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
			text: `${regimeLabel(stop.Regime)} trip — desk should exit on this mark`,
			tone: "bad",
		};
	}

	if (stop.State === STOPLOSS_ARMED) {
		const proximity = stopProximity(stop);
		const confirms = stop.Confirms ?? 0;
		const needed = confirmsNeeded(stop);
		return {
			text: `${regimeLabel(stop.Regime)} armed — mark in the breathing room`,
			tone: proximity >= 0.7 || confirms >= needed ? "warn" : "info",
		};
	}

	if (stop.PeakPnLPct >= stop.ArmPnlPct) {
		return {
			text: "Peak cleared arm — waiting for next mark to arm",
			tone: "good",
		};
	}

	return {
		text: "Climbing toward the arm line",
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
	const needed = confirmsNeeded(stop);
	const exh = stop.ExhaustScore ?? 0;
	const confirmPct = Math.max(0, Math.min(100, (confirms / needed) * 100));
	const floor = givebackFloor(stop);

	return [
		{
			id: "crest",
			name: "Crest",
			role: "defence",
			word: {
				label: armed ? `${room.toFixed(1)}pp to floor` : "not armed",
				short: armed ? `${proxPct.toFixed(0)}%` : "—",
				title: `Trailing floor ${floor.toFixed(2)}%`,
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
				label: armed ? `c${confirms}/${needed} exh${exh.toFixed(2)}` : "—",
				short: armed ? `${confirmPct.toFixed(0)}%` : "—",
				title: "Exit confirms × climb exhaust",
			},
			percent: armed ? confirmPct : null,
			tone:
				confirms >= needed
					? "bad"
					: confirms >= 1 || exh >= 0.7
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
