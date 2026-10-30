/*
Human labels for desk and model exit triggers surfaced in lifecycle and positions.

The trigger strings are the *first* token of the exit policy message the
StopLoss and gate emit. Keep them stable: the dashboard paints from them and
a renamed string silently degrades a labelled exit into raw text.
*/
export const EXIT_TRIGGER_LABEL: Record<string, string> = {
	// Legacy desk/model triggers.
	learned_exit: "EXIT",
	capacity_trim: "TRIM",
	capacity_exit: "CAP-EXIT",
	manual: "MANUAL",
	emergency_support_evaporation: "EMERGENCY EVAC",
	timeout_stall: "TIMEOUT STALL",
	thin_book_fallback: "THIN BOOK",
	"desk exit": "DESK EXIT",
	book: "BOOK HURDLE",
	"cleared friction": "PROFIT HURDLE",
	"friction dip": "STOP HURDLE",
	"exit branch": "EXIT BRANCH",
	"volatility floor breached": "VOL FLOOR",
	"pct profit floor": "PCT FLOOR",
	"profit stagnation": "PROFIT STALL",
	"bid floor evaporated": "BID PULL",
	"spike retrace": "SPIKE RETRACE",
	"statistical invalidation stop": "STAT STOP",
	"entry drag exceeded": "DRAG STOP",
	// broker/stoploss.go check 1: causal-MCTS learned invalidation cut.
	"invalidation stop": "STOP",
	// broker/stoploss.go check 2: physical book liquidity evaporation (3-tick persistence).
	"liquidity evaporation": "EVAP",
	// broker/stoploss.go check 3: causal-MCTS learned trailing / ratcheted profit stop.
	"ratcheted trailing stop": "TRAIL",
	"damaged recovery ratcheted stop": "TRAIL",
	// broker/stoploss.go check 4: empirical move expiry (bar-count, tape timebase).
	"move expired": "EXPIRE",
	// Gate exit path.
	"policy exit match": "EXIT",
};

/*
formatExitTrigger turns a raw trigger string into a short label. Unknown
strings fall back to the text with underscores spaced, so a renamed trigger
never vanishes — it reads a little worse but stays present.
*/
export const formatExitTrigger = (trigger: string): string => {
	if (EXIT_TRIGGER_LABEL[trigger]) {
		return EXIT_TRIGGER_LABEL[trigger];
	}
	const lower = trigger.toLowerCase();
	for (const [key, label] of Object.entries(EXIT_TRIGGER_LABEL)) {
		if (lower.startsWith(key)) {
			return label;
		}
	}
	return trigger.replaceAll("_", " ");
};

/*
triggerTone paints a trigger by what it means, so a row of exits is
self-evident: a harvested exit is good, a stop-out is bad, an expiry is a
warn. Unknown is neutral.
*/
export const triggerTone = (trigger: string): "good" | "bad" | "warn" | "neutral" => {
	const label = trigger.toLowerCase();
	if (
		label.includes("invalidation") ||
		label.includes("evaporation") ||
		label.includes("stop") && !label.includes("trailing")
	) {
		return "bad";
	}
	if (label.includes("trailing") || label.includes("harvest") || label.includes("profit")) {
		return "good";
	}
	if (label.includes("expired") || label.includes("expire")) {
		return "warn";
	}
	return "neutral";
};

export const BLOCKED_REASON_LABEL: Record<string, string> = {
	ignition_sell_side: "R07 blocked (sell-side / bearish flow)",
	enter_vetoed: "Enter vetoed (opposition)",
	exit_vetoed: "Exit vetoed",
	no_side: "No enter/exit vote",
	capacity_risk: "Capacity risk (R14)",
	hurdle: "Below hurdle",
};

export const formatBlockedReason = (reason: string): string =>
	BLOCKED_REASON_LABEL[reason] ?? reason.replaceAll("_", " ");
