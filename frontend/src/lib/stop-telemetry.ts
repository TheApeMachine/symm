/*
Human words and tones for the simplified stop-loss checks so a held lot is
never a black box. Each helper reads the live proximity the desk publishes
(broker.StopLoss.State) and says what it means: what check is armed, how close
it is to firing, and whether the lot is safe.

The two checks (broker/stoploss.go Update), in priority order:
  1. invalidation — adverse drop from entry vs a fixed 2.0pp cut (defence)
  2. trailing     — giveback from peak vs a damage-ratcheted trail (harvest)

Damage (0–1) is how much of the 2pp invalidation budget the worst print burned;
it tightens the trail but is not itself an exit.
*/

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

export type MeterTone =
	| "success"
	| "warning"
	| "error"
	| "info"
	| "disabled";

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

export const proximityPct = (ratio: number | null | undefined): number | null => {
	if (ratio === null || ratio === undefined || !Number.isFinite(ratio)) {
		return null;
	}
	return Math.max(0, Math.min(100, ratio * 100));
};

const defenceTone = (ratio: number | null | undefined): Tone => {
	if (ratio === null || ratio === undefined || !Number.isFinite(ratio)) {
		return "neutral";
	}
	if (ratio >= 1) return "bad";
	if (ratio >= 0.7) return "warn";
	if (ratio >= 0.4) return "info";
	return "good";
};

const harvestTone = (ratio: number | null | undefined): Tone => {
	if (ratio === null || ratio === undefined || !Number.isFinite(ratio)) {
		return "neutral";
	}
	if (ratio >= 1) return "warn";
	if (ratio >= 0.6) return "info";
	return "good";
};

export type StopWord = { label: string; short: string; title: string };

export const heldVerdict = (args: {
	current: number | null;
	peak: number | null;
	invalidation: number | null;
	trailing: number | null;
	damage?: number | null;
	giveback?: number | null;
	allowedGiveback?: number | null;
	remainingGiveback?: number | null;
}): { text: string; tone: Tone } => {
	const { current, peak, invalidation, trailing } = args;

	const currentKnown = current !== null && Number.isFinite(current);
	const peakKnown = peak !== null && Number.isFinite(peak);

	if ((invalidation ?? 0) >= 1) {
		return {
			text: "Invalidation cut reached — stop loss arming",
			tone: "bad",
		};
	}
	if ((trailing ?? 0) >= 1) {
		return {
			text: "Trailing giveback reached — harvesting profit",
			tone: "warn",
		};
	}

	if (!currentKnown) {
		return { text: "Waiting for the desk book to price the lot", tone: "neutral" };
	}

	if (peakKnown && peak !== null && peak > 0.5) {
		if ((trailing ?? 0) >= 0.999 || (args.remainingGiveback ?? 1) <= 0.001) {
			return {
				text: "Trail hit — desk should exit on this mark",
				tone: "warn",
			};
		}

		const room = args.remainingGiveback;
		if (room !== null && room !== undefined && Number.isFinite(room)) {
			const damage =
				args.damage !== null &&
				args.damage !== undefined &&
				Number.isFinite(args.damage) &&
				args.damage > 0
					? `, damage ${Math.round(args.damage * 100)}%`
					: "";
			return {
				text: `In profit — can drop ${Math.max(0, room).toFixed(1)}% more before trail exit${damage}`,
				tone: room <= (args.allowedGiveback ?? room) * 0.25 ? "warn" : "good",
			};
		}
		return {
			text: `In profit (peak ${peak.toFixed(1)}%) — trail exit armed`,
			tone: "good",
		};
	}

	if (invalidation !== null && invalidation !== undefined && invalidation > 0) {
		return {
			text: `Underwater, ${Math.round(invalidation * 100)}% of the 2pp stop budget consumed`,
			tone: invalidation >= 0.7 ? "bad" : invalidation >= 0.4 ? "warn" : "info",
		};
	}

	return {
		text: `Holding near entry (${current.toFixed(2)}%) — stops armed, far from firing`,
		tone: "neutral",
	};
};

export type StopCheck = {
	id: "invalidation" | "trailing" | "damage";
	name: string;
	role: "defence" | "harvest" | "context";
	word: StopWord;
	percent: number | null;
	tone: Tone;
};

const fmtPct = (value: number | null): string =>
	value === null || !Number.isFinite(value) ? "—" : `${value.toFixed(1)}%`;

export const invalidationCheck = (
	ratio: number | null | undefined,
	current: number | null | undefined,
): StopCheck => {
	const armed =
		ratio !== null && ratio !== undefined && Number.isFinite(ratio) && ratio > 0;
	const pct = proximityPct(ratio);
	return {
		id: "invalidation",
		name: "Stop loss",
		role: "defence",
		word: {
			label: "Stop loss",
			short: armed ? `${Math.round(pct ?? 0)}% used` : "far",
			title: `Hard invalidation: adverse drop from the entry mark vs a fixed 2.0pp cut. ${
				armed
					? `Now ${Math.round((pct ?? 0) * 100) / 100}% of the budget consumed${
							current !== undefined && current !== null
								? `, current ${fmtPct(current)}`
								: ""
						}`
					: "No adverse move against the cut yet."
			}`,
		},
		percent: pct,
		tone: defenceTone(ratio),
	};
};

export const trailingCheck = (
	ratio: number | null | undefined,
	peak: number | null | undefined,
	remainingGiveback: number | null | undefined,
	allowedGiveback: number | null | undefined,
	giveback: number | null | undefined,
	current: number | null | undefined,
): StopCheck => {
	const green =
		peak !== null && peak !== undefined && Number.isFinite(peak) && peak > 0.5;
	const room =
		remainingGiveback !== null &&
		remainingGiveback !== undefined &&
		Number.isFinite(remainingGiveback)
			? remainingGiveback
			: null;
	const allowed =
		allowedGiveback !== null &&
		allowedGiveback !== undefined &&
		Number.isFinite(allowedGiveback)
			? allowedGiveback
			: null;
	const pct = proximityPct(ratio);

	const atTrail =
		(ratio !== null &&
			ratio !== undefined &&
			Number.isFinite(ratio) &&
			ratio >= 0.999) ||
		(room !== null && room <= 0.001);

	let short = "flat";
	if (green && atTrail) {
		short = "exit now";
	} else if (green && room !== null) {
		short = `${Math.max(0, room).toFixed(1)}% until exit`;
	} else if (green && allowed !== null) {
		short = `${allowed.toFixed(1)}% max drop`;
	}

	const detail =
		green && allowed !== null
			? `Peak ${fmtPct(peak)}, now ${fmtPct(current ?? null)}. Already gave back ${fmtPct(giveback ?? null)}. Trail exits after ${allowed.toFixed(1)}% off the peak${
					room !== null ? ` — ${room.toFixed(1)}% left.` : "."
				}`
			: "Arms once peak clears +0.5% and the lot is still green.";

	return {
		id: "trailing",
		name: "Trail",
		role: "harvest",
		word: {
			label: "Trail",
			short,
			title: detail,
		},
		percent: pct,
		tone: harvestTone(ratio),
	};
};

export const damageCheck = (ratio: number | null | undefined): StopCheck => {
	const armed =
		ratio !== null && ratio !== undefined && Number.isFinite(ratio) && ratio > 0;
	const pct = proximityPct(ratio);
	return {
		id: "damage",
		name: "Damage",
		role: "context",
		word: {
			label: "Damage",
			short: armed ? `${Math.round(pct ?? 0)}%` : "clean",
			title: armed
				? `Worst drawdown from entry has burned ${Math.round(pct ?? 0)}% of the 2pp invalidation budget — trail giveback tightens with damage.`
				: "No adverse excursion yet; trail uses the full base giveback room.",
		},
		percent: pct,
		tone:
			!armed
				? "neutral"
				: (ratio ?? 0) >= 0.8
					? "warn"
					: (ratio ?? 0) >= 0.4
						? "info"
						: "good",
	};
};

/*
buildStopChecks assembles invalidation, trailing, and damage from a live stop
state. Peak/current come from stop_state (falling back to lifecycle fields).
*/
export const buildStopChecks = (args: {
	invalidation: number | null | undefined;
	trailing: number | null | undefined;
	damage?: number | null | undefined;
	current?: number | null | undefined;
	peak?: number | null | undefined;
	giveback?: number | null | undefined;
	allowedGiveback?: number | null | undefined;
	remainingGiveback?: number | null | undefined;
}): StopCheck[] => {
	return [
		invalidationCheck(args.invalidation, args.current),
		trailingCheck(
			args.trailing,
			args.peak,
			args.remainingGiveback,
			args.allowedGiveback,
			args.giveback,
			args.current,
		),
		damageCheck(args.damage),
	];
};

/* Fixed stop rule rendered as a fact on the position card. */
export const stopRule = (
	label: string,
	short: string,
	title: string,
): StopWord => ({ label, short, title });
