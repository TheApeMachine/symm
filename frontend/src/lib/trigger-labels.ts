/*
Human labels for desk and model exit triggers surfaced in lifecycle and positions.
*/
export const EXIT_TRIGGER_LABEL: Record<string, string> = {
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
};

export const formatExitTrigger = (trigger: string): string =>
	EXIT_TRIGGER_LABEL[trigger] ?? trigger.replaceAll("_", " ");

export const BLOCKED_REASON_LABEL: Record<string, string> = {
	ignition_sell_side: "R07 blocked (sell-side / bearish flow)",
	enter_vetoed: "Enter vetoed (opposition)",
	exit_vetoed: "Exit vetoed",
	no_side: "No enter/exit vote",
};

export const formatBlockedReason = (reason: string): string =>
	BLOCKED_REASON_LABEL[reason] ?? reason.replaceAll("_", " ");
