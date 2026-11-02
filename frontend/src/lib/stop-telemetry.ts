/*
Human words and tones for DynamicStoploss so a held lot is never a black box.
State comes from broker.DynamicStoploss on each position frame.
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
		case "Protected":
			return "info";
		case "Dangerous":
		case "Critical":
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
	downside_pressure?: number | null;
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

	if (args.state === "Dangerous") {
		return {
			text: "Dangerous — floor proximity or cascade pressure is elevated",
			tone: "bad",
		};
	}

	if (args.state === "Critical") {
		return {
			text: "Critical — room to floor is thinner than entry drag",
			tone: "bad",
		};
	}

	const current = args.current;
	if (current === null || !Number.isFinite(current)) {
		return {
			text: "Waiting for the desk book to price the lot",
			tone: "neutral",
		};
	}

	if (
		args.has_floor &&
		args.proximity !== null &&
		args.proximity !== undefined
	) {
		if (args.proximity >= 1) {
			return { text: "At the profit floor — stop should arm", tone: "bad" };
		}

		const room =
			args.room_pct !== null &&
			args.room_pct !== undefined &&
			Number.isFinite(args.room_pct)
				? `${Math.abs(args.room_pct).toFixed(1)}pp room`
				: null;
		if (room) {
			return {
				text: `Protected — ${room} before floor at ${fmtPct(args.floor)}`,
				tone: args.proximity >= 0.7 ? "warn" : stateTone(args.state),
			};
		}
	}

	const peakKnown = args.peak !== null && Number.isFinite(args.peak);
	if (peakKnown && args.peak !== null && args.peak > (args.friction ?? 0)) {
		return {
			text: `In profit (peak ${args.peak.toFixed(1)}%) — dynamic floor armed`,
			tone: "good",
		};
	}

	if (
		args.downside_pressure !== null &&
		args.downside_pressure !== undefined &&
		args.downside_pressure > 0
	) {
		return {
			text: `Underwater — downside pressure ${Math.round(args.downside_pressure * 100)}%`,
			tone: args.downside_pressure >= 0.7 ? "bad" : "warn",
		};
	}

	return {
		text: `Holding near entry (${current.toFixed(2)}%) — stop healthy`,
		tone: stateTone(args.state) === "neutral" ? "good" : stateTone(args.state),
	};
};

export type StopCheck = {
	id: "floor" | "pressure" | "stall";
	name: string;
	role: "defence" | "harvest" | "context";
	word: StopWord;
	percent: number | null;
	tone: Tone;
};

export type StopWord = { label: string; short: string; title: string };

const pressureTone = (ratio: number | null | undefined): Tone => {
	if (ratio === null || ratio === undefined || !Number.isFinite(ratio)) {
		return "neutral";
	}
	if (ratio >= 0.85) return "bad";
	if (ratio >= 0.55) return "warn";
	if (ratio >= 0.25) return "info";
	return "good";
};

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
	downside_pressure?: number | null;
	downside_magnitude_ratio?: number | null;
	consecutive_down?: number | null;
	has_floor?: boolean;
	room_pct?: number | null;
	current?: number | null;
}): StopCheck[] => {
	const prox = proximityPct(args.proximity);
	const pressure = proximityPct(args.downside_pressure);

	const floorShort =
		args.has_floor && args.room_pct !== null && args.room_pct !== undefined
			? `${Math.max(0, args.room_pct).toFixed(1)}pp room`
			: args.has_floor
				? prox !== null
					? `${Math.round(prox)}% to floor`
					: "armed"
				: "no floor";

	const checks: StopCheck[] = [
		{
			id: "floor",
			name: "Floor",
			role: "harvest",
			word: {
				label: "Floor proximity",
				short: floorShort,
				title: args.has_floor
					? `Distance from peak toward the profit floor. ${prox !== null ? `${Math.round(prox)}% of the peak→floor span consumed` : "Floor is set"}${args.current !== null && args.current !== undefined ? `, mark ${fmtPct(args.current)}` : ""}.`
					: "Floor has not armed yet — trail engages once peak clears friction.",
			},
			percent: args.has_floor ? prox : null,
			tone: args.has_floor ? floorTone(args.proximity) : "neutral",
		},
		{
			id: "pressure",
			name: "Pressure",
			role: "defence",
			word: {
				label: "Downside pressure",
				short: pressure !== null ? `${Math.round(pressure)}%` : "calm",
				title:
					pressure !== null
						? `Cascade pressure from consecutive adverse marks — ${Math.round(pressure)}% of the exit budget.`
						: "No elevated downside pressure on this mark.",
			},
			percent: pressure,
			tone: pressureTone(args.downside_pressure),
		},
		{
			id: "stall",
			name: "Stall",
			role: "context",
			word: {
				label: "Stall",
				short:
					args.state === "Critical"
						? "critical"
						: typeof args.consecutive_down === "number" &&
								args.consecutive_down > 0
							? `${args.consecutive_down}↓`
							: "clear",
				title:
					args.state === "Critical"
						? "Room to floor is thinner than entry drag."
						: typeof args.consecutive_down === "number" &&
								args.consecutive_down > 0
							? `${args.consecutive_down} consecutive down marks without a new peak.`
							: "No stall pattern on the current path.",
			},
			percent:
				args.state === "Critical"
					? 90
					: typeof args.consecutive_down === "number" &&
							args.consecutive_down > 0
						? Math.min(100, args.consecutive_down * 20)
						: null,
			tone:
				args.state === "Critical"
					? "bad"
					: typeof args.consecutive_down === "number" &&
							args.consecutive_down >= 3
						? "info"
						: "neutral",
		},
	];

	return checks;
};

export const stopRule = (
	label: string,
	short: string,
	title: string,
): StopWord => ({ label, short, title });
