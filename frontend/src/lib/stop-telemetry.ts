/*
Human words and tones for the stop-loss checks and the learned exit policy, so
a held lot is never a black box. Each helper reads the live proximity the desk
publishes (broker.StopLoss.State) or a learned boundary and says what it means:
what check is armed, how close it is to firing, and whether the lot is safe.

The four checks, in priority order (broker/stoploss.go Evaluate):
  1. invalidation  — adverse drop below entry vs the learned cut (defence)
  2. evaporation   — full-lot slippage doubles friction, 3-tick persistence
  3. trailing      — giveback from the peak vs the learned trail (harvest)
  4. expiry        — bars held past the empirical winning horizon
*/

/* A neutral reading that a meter or badge can paint. */
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

/* Meter fill variants match the UI library so stops paint the same way. */
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

/* A ratio is current-value-over-threshold. A null/undefined ratio means the
   check is not armed (no price, no learned boundary, or below its trigger). */
export const proximityPct = (ratio: number | null | undefined): number | null => {
	if (ratio === null || ratio === undefined || !Number.isFinite(ratio)) {
		return null;
	}
	return Math.max(0, Math.min(100, ratio * 100));
};

/* A defence check's tone flips the meaning: far is safe, close is bad. */
const defenceTone = (ratio: number | null | undefined): Tone => {
	if (ratio === null || ratio === undefined || !Number.isFinite(ratio)) {
		return "neutral";
	}
	if (ratio >= 1) return "bad";
	if (ratio >= 0.7) return "warn";
	if (ratio >= 0.4) return "info";
	return "good";
};

/* A harvest check only matters once the lot is green; a null ratio (not yet
   in profit, or peak never exceeded friction) reads armed-but-far. */
const harvestTone = (ratio: number | null | undefined): Tone => {
	if (ratio === null || ratio === undefined || !Number.isFinite(ratio)) {
		return "neutral";
	}
	if (ratio >= 1) return "warn";
	if (ratio >= 0.6) return "info";
	return "good";
};

export type StopWord = { label: string; short: string; title: string };

/* The one-sentence story a held lot tells: the single most relevant check and
   why. This is the "why is it still held, or about to exit" line. */
export const heldVerdict = (args: {
	current: number | null;
	peak: number | null;
	invalidation: number | null;
	evaporation: number | null;
	trailing: number | null;
	expiry: number | null;
	barsHeld?: number | null;
	maxBars?: number | null;
}): { text: string; tone: Tone } => {
	const {
		current,
		peak,
		invalidation,
		evaporation,
		trailing,
		expiry,
		barsHeld,
		maxBars,
	} = args;

	const currentKnown = current !== null && Number.isFinite(current);
	const peakKnown = peak !== null && Number.isFinite(peak);

	// A check already firing means the exit is in motion.
	if ((evaporation ?? 0) >= 1) {
		return {
			text: "Book pulling — liquidity evaporation armed, exit in motion",
			tone: "bad",
		};
	}
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

	// Expiry only arms on a lot that has never gone green.
	if (peakKnown && peak !== null && peak <= 0) {
		if ((expiry ?? 0) >= 1) {
			return {
				text: "Move expired — horizon passed without profit",
				tone: "warn",
			};
		}
		if (
			expiry !== null &&
			expiry !== undefined &&
			barsHeld !== null &&
			maxBars !== null &&
			maxBars > 0
		) {
			return {
				text: `Holding ${barsHeld}/${maxBars} bars, no profit yet — expiry ${Math.round(
					expiry * 100,
				)}% used`,
				tone: expiry >= 0.7 ? "warn" : "info",
			};
		}
	}

	if (!currentKnown) {
		return { text: "Waiting for the desk book to price the lot", tone: "neutral" };
	}

	// Trailing is the live story once green.
	if (peakKnown && peak !== null && peak > 0) {
		if (trailing !== null && trailing !== undefined && trailing > 0) {
			return {
				text: `In profit, giveback ${Math.round(trailing * 100)}% of the trail — trail holds the peak`,
				tone: trailing >= 0.6 ? "warn" : "good",
			};
		}
		return {
			text: `Ahead ${peak.toFixed(2)}%, holding — trailing stop watching the peak`,
			tone: "good",
		};
	}

	// Underwater: the invalidation budget is the story.
	if (invalidation !== null && invalidation !== undefined && invalidation > 0) {
		return {
			text: `Underwater, ${Math.round(invalidation * 100)}% of the stop budget consumed — stop armed`,
			tone: invalidation >= 0.7 ? "bad" : invalidation >= 0.4 ? "warn" : "info",
		};
	}

	return {
		text: `Holding near entry (${current.toFixed(2)}%) — stops armed, far from firing`,
		tone: "neutral",
	};
};

/* One row of the four checks, ready for a meter. The word states the rule and
   the current distance so a glance says why. */
export type StopCheck = {
	id: "invalidation" | "evaporation" | "trailing" | "expiry";
	name: string;
	role: "defence" | "harvest" | "time";
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
			short: armed ? `${Math.round(pct ?? 0)}% used` : "far",
			title: `Causal-MCTS invalidation cut: adverse drop below entry vs the learned cut. ${
				armed
					? `Now ${Math.round((pct ?? 0) * 100) / 100} of the cut budget consumed${
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

export const evaporationCheck = (ratio: number | null | undefined): StopCheck => {
	const armed =
		ratio !== null && ratio !== undefined && Number.isFinite(ratio) && ratio > 0;
	const pct = proximityPct(ratio);
	return {
		id: "evaporation",
		name: "Liquidity",
		role: "defence",
		word: {
			short: armed ? `${Math.round(pct ?? 0)}%` : "liquid",
			title: `Full-lot exit slippage vs 2× round-trip friction. Fires on a book that stays pulled 3 marks in a row, regardless of P&L — this is why a thin book is a real exit, not a flicker.`,
		},
		percent: pct,
		tone: defenceTone(ratio),
	};
};

export const trailingCheck = (
	ratio: number | null | undefined,
	peak: number | null | undefined,
): StopCheck => {
	const armed =
		peak !== null &&
		peak !== undefined &&
		peak > 0 &&
		ratio !== null &&
		ratio !== undefined &&
		Number.isFinite(ratio) &&
		ratio > 0;
	const pct = proximityPct(ratio);
	return {
		id: "trailing",
		name: "Trail",
		role: "harvest",
		word: {
			short: armed
				? `${Math.round(pct ?? 0)}% back`
				: peak !== null && peak !== undefined && peak > 0
					? "at peak"
					: "not green",
			title: `Causal-MCTS trailing stop: giveback from the peak vs the learned pullback, ratcheted tighter the larger the peak. ${
				peak !== null && peak !== undefined && peak > 0
					? `Peak ${fmtPct(peak)}${
							armed ? `, now ${Math.round((pct ?? 0) * 100) / 100} of the trail given back` : ", holding near peak"
						}`
					: "Arms once the lot has been in profit."
			}`,
		},
		percent: armed ? pct : null,
		tone: harvestTone(ratio),
	};
};

export const expiryCheck = (
	ratio: number | null | undefined,
	barsHeld: number | null | undefined,
	maxBars: number | null | undefined,
	peak: number | null | undefined,
): StopCheck => {
	const pct = proximityPct(ratio);
	// Expiry only arms on a move that never went green, so a null peak or a
	// green peak reads neutral: time is not the exit pressure on a winner.
	const neverGreen = peak !== null && peak !== undefined && peak <= 0;
	return {
		id: "expiry",
		name: "Expiry",
		role: "time",
		word: {
			short:
				maxBars && maxBars > 0 && barsHeld !== null && barsHeld !== undefined
					? `${Math.min(barsHeld, maxBars)}/${maxBars} bars`
					: pct !== null
						? `${Math.round(pct)}%`
						: "—",
			title: `Move expiry: bars held past the empirical winning horizon of winning excursions. Arms only while the lot has never been in profit — a winning move is never time-cut.`,
		},
		percent: pct,
		tone: neverGreen ? defenceTone(ratio) : "neutral",
	};
};

/*
buildStopChecks assembles the four checks in priority order from a live stop
state. Peak/current come from stop_state (falling back to the lifecycle fields)
so an open lot and a restored closed lot both render the same way.
*/
export const buildStopChecks = (args: {
	invalidation: number | null | undefined;
	evaporation: number | null | undefined;
	trailing: number | null | undefined;
	expiry: number | null | undefined;
	current?: number | null | undefined;
	peak?: number | null | undefined;
	barsHeld?: number | null | undefined;
	maxBars?: number | null | undefined;
}): StopCheck[] => {
	return [
		invalidationCheck(args.invalidation, args.current),
		evaporationCheck(args.evaporation),
		trailingCheck(args.trailing, args.peak),
		expiryCheck(args.expiry, args.barsHeld, args.maxBars, args.peak),
	];
};

/* A learned boundary rendered as a fact: the threshold the stop is actually
   enforcing, and whether it is learned (positive) or still at the friction
   floor. A null is "not learned yet" and paints neutral, never a fake 0%. */
export const policyBoundary = (
	label: string,
	value: number | null | undefined,
	option?: { fraction?: boolean },
): StopWord => {
	const learned = value !== null && value !== undefined && Number.isFinite(value);
	const text = !learned
		? "not learned"
		: option?.fraction
			? `${(100 * value).toFixed(1)}%`
			: `${value.toFixed(1)}%`;
	return {
		label,
		short: text,
		title: learned
			? `${label}: ${text} — derived by Causal MCTS from this symbol's settled excursions`
			: `${label}: the desk has not learned this boundary yet; the stop falls back to the friction floor`,
	};
};
