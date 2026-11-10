/* Confluence region taxonomy used by impulse map and training UI. */

export interface RegionMeta {
	id: string;
	title: string;
	subtitle: string;
	role:
		| "entry_detector"
		| "entry_support"
		| "entry_opposition"
		| "exit_detector"
		| "exit_support"
		| "exit_opposition"
		| "hold_detector"
		| "hold_support"
		| "hold_opposition"
		| "oracle_veto"
		| "oracle_approve";
	tone: "up" | "down" | "acc" | "info" | "neutral" | "warn";
	description: string;
}

export const REGIONS: Record<string, RegionMeta> = {
	R00: {
		id: "R00",
		title: "Enter detector",
		subtitle: "Coiled compression",
		role: "entry_detector",
		tone: "up",
		description:
			"Quiet tape with rising participation — volume rate over baseline while midpoint barely moves.",
	},
	R01: {
		id: "R01",
		title: "Enter support",
		subtitle: "Structural alignment",
		role: "entry_support",
		tone: "up",
		description:
			"Board-wide peer reprices moving together; enter with the cohort, not a lone name.",
	},
	R02: {
		id: "R02",
		title: "Enter opposition",
		subtitle: "Starved tape",
		role: "entry_opposition",
		tone: "info",
		description:
			"Participation collapsed vs this symbol’s recent norm — flat lines and fake tries.",
	},
	R03: {
		id: "R03",
		title: "Exit detector",
		subtitle: "Rejection tick",
		role: "exit_detector",
		tone: "down",
		description:
			"Down bar on the volume clock — midpoint rejected lower on the closed bar.",
	},
	R04: {
		id: "R04",
		title: "Exit support",
		subtitle: "Bid ladder release",
		role: "exit_support",
		tone: "down",
		description:
			"Displayed bid depth peeling off between L3 snapshots — sell while bids are still shown.",
	},
	R05: {
		id: "R05",
		title: "Exit opposition",
		subtitle: "Ask ladder release",
		role: "exit_opposition",
		tone: "down",
		description:
			"Displayed ask depth peeling off between L3 snapshots — thinning offers, precursor that the leg may still have room.",
	},
	R06: {
		id: "R06",
		title: "Hold detector",
		subtitle: "Supercritical cascade",
		role: "hold_detector",
		tone: "neutral",
		description:
			"Hawkes branching at or above criticality — self-exciting trade feedback on the tape.",
	},
	R07: {
		id: "R07",
		title: "Hold support",
		subtitle: "Bid ladder build",
		role: "hold_support",
		tone: "acc",
		description:
			"Net bid size stacking in on the visible ladder — book building under the mark.",
	},
	R08: {
		id: "R08",
		title: "Hold opposition",
		subtitle: "Blowoff dip",
		role: "hold_opposition",
		tone: "warn",
		description:
			"Strong down bar after a remembered strong up bar — blow-off pullback while still holding.",
	},
	R09: {
		id: "R09",
		title: "Volume quote veto",
		subtitle: "24h turnover floor",
		role: "oracle_veto",
		tone: "warn",
		description: "Quote volume below the mover floor — name is not tradable.",
	},
	R10: {
		id: "R10",
		title: "Volume quote approve",
		subtitle: "24h turnover clear",
		role: "oracle_approve",
		tone: "up",
		description: "Enough quote turnover and a known instrument.",
	},
	R11: {
		id: "R11",
		title: "Day move veto",
		subtitle: "Flat open-to-last",
		role: "oracle_veto",
		tone: "warn",
		description: "Open-to-last change below the mover floor — liquid but idle.",
	},
	R12: {
		id: "R12",
		title: "Day move approve",
		subtitle: "Open-to-last clear",
		role: "oracle_approve",
		tone: "up",
		description: "Day move clears the floor — name is alive enough to touch.",
	},
	R13: {
		id: "R13",
		title: "Spread veto",
		subtitle: "Width / scarcity",
		role: "oracle_veto",
		tone: "warn",
		description: "Touch spread above cap or liquidity scarcity extreme.",
	},
	R14: {
		id: "R14",
		title: "Spread approve",
		subtitle: "Tight / median depth",
		role: "oracle_approve",
		tone: "up",
		description: "Executable width and compressed spread vs the board.",
	},
	R15: {
		id: "R15",
		title: "Book integrity veto",
		subtitle: "Cash / backing / tape",
		role: "oracle_veto",
		tone: "warn",
		description: "Touch cash, backing, tape uptake, or open drag fails.",
	},
	R16: {
		id: "R16",
		title: "Book integrity approve",
		subtitle: "Backing / depth",
		role: "oracle_approve",
		tone: "up",
		description: "Backing firm with toxicity support and depth score.",
	},
	R17: {
		id: "R17",
		title: "Board confidence veto",
		subtitle: "Board stress",
		role: "oracle_veto",
		tone: "warn",
		description: "Board median stress, slump, or exhaustion.",
	},
	R18: {
		id: "R18",
		title: "Board confidence approve",
		subtitle: "Board story lit",
		role: "oracle_approve",
		tone: "up",
		description: "Board median herd, compression, cascade, surge alignment.",
	},
	R19: {
		id: "R19",
		title: "Contagion veto",
		subtitle: "Lead break / stress",
		role: "oracle_veto",
		tone: "warn",
		description: "Lead-lag decouple/stall or cohort stress/noise.",
	},
	R20: {
		id: "R20",
		title: "Contagion approve",
		subtitle: "Sync / herd",
		role: "oracle_approve",
		tone: "up",
		description: "Lead structure intact — sync, herd, alpha.",
	},
	R21: {
		id: "R21",
		title: "Capacity veto",
		subtitle: "Thin / vacuum",
		role: "oracle_veto",
		tone: "warn",
		description: "Scarcity, vacuum, thin book, or exit impact too high.",
	},
	R22: {
		id: "R22",
		title: "Capacity approve",
		subtitle: "Depth clear",
		role: "oracle_approve",
		tone: "up",
		description: "Depth and median liquidity can carry the desk lot.",
	},
	R23: {
		id: "R23",
		title: "24h peak veto",
		subtitle: "Top of day range",
		role: "oracle_veto",
		tone: "warn",
		description:
			"Last sits at or above the peak fraction of the 24h high–low range — chasing the high.",
	},
	R24: {
		id: "R24",
		title: "24h peak approve",
		subtitle: "Room in day range",
		role: "oracle_approve",
		tone: "up",
		description:
			"Last is still under the peak fraction of the 24h range — not buying the top.",
	},
};

export const REGION_KEYS = Object.keys(REGIONS);
