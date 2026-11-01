import { createStore } from "@tanstack/store";

export type OracleOverride = "auto" | "veto" | "approve";

export type OracleGate = {
	id: string;
	label: string;
	veto: number;
	approve: number;
	/** Short plain-language what / why for tooltips. */
	hint: string;
};

export const ORACLE_GATES: OracleGate[] = [
	{
		id: "vol",
		label: "Volume quote",
		veto: 9,
		approve: 10,
		hint: "Is 24h quote turnover big enough to bother? Veto = too quiet; approve = liquid enough.",
	},
	{
		id: "day",
		label: "Day move",
		veto: 11,
		approve: 12,
		hint: "Did price actually move today? Veto = flat/open-to-last too small; approve = alive.",
	},
	{
		id: "spr",
		label: "Spread",
		veto: 13,
		approve: 14,
		hint: "Is the touch spread tradable? Veto = too wide / scarce; approve = tight enough to enter.",
	},
	{
		id: "bok",
		label: "Book integrity",
		veto: 15,
		approve: 16,
		hint: "Can the book carry a lot? Cash, backing, tape liveness, open drag. Veto = thin/fake; approve = firm.",
	},
	{
		id: "brd",
		label: "Board confidence",
		veto: 17,
		approve: 18,
		hint: "Does the whole board agree with this story? Veto = stress/slump; approve = herd/cascade/surge.",
	},
	{
		id: "ctg",
		label: "Contagion",
		veto: 19,
		approve: 20,
		hint: "Is lead structure breaking? Veto = decouple/stall/stress; approve = sync/herd still intact.",
	},
	{
		id: "cap",
		label: "Capacity",
		veto: 21,
		approve: 22,
		hint: "Can we exit without blowing impact? Veto = thin/vacuum; approve = depth can carry the size.",
	},
];

export type RiskDraft = {
	min_mover_volume_usd: number;
	min_mover_change_pct: number;
	max_entry_spread_pct: number;
	min_touch_bid_cash: number;
	min_backing_ratio: number;
	min_tape_trades: number;
	max_entry_open_drag_pct: number;
};

export const RISK_DEFAULTS: RiskDraft = {
	min_mover_volume_usd: 100_000,
	min_mover_change_pct: 0.5,
	max_entry_spread_pct: 2.0,
	min_touch_bid_cash: 100,
	min_backing_ratio: 0.6,
	min_tape_trades: 8,
	max_entry_open_drag_pct: 2.0,
};

export type ResonanceDraft = {
	bypass: boolean;
	min_task_relative_precision: number;
	min_surprise: number;
	min_supported_horizon: number;
	min_enter_z: number;
};

export const RESONANCE_DEFAULTS: ResonanceDraft = {
	bypass: false,
	min_task_relative_precision: 0.5,
	min_surprise: 0.5,
	min_supported_horizon: 2,
	min_enter_z: 0.35,
};

export type GateLabState = {
	open: boolean;
	overrides: Record<string, OracleOverride>;
	risk: RiskDraft;
	resonance: ResonanceDraft;
};

const emptyOverrides = (): Record<string, OracleOverride> => {
	const out: Record<string, OracleOverride> = {};
	for (const gate of ORACLE_GATES) out[gate.id] = "auto";
	return out;
};

export const gateLabStore = createStore<GateLabState>({
	open: false,
	overrides: emptyOverrides(),
	risk: { ...RISK_DEFAULTS },
	resonance: { ...RESONANCE_DEFAULTS },
});

export const toggleGateLab = () => {
	gateLabStore.setState((prev) => ({ ...prev, open: !prev.open }));
};

const setOracleOverride = (id: string, value: OracleOverride) => {
	gateLabStore.setState((prev) => ({
		...prev,
		overrides: { ...prev.overrides, [id]: value },
	}));
};

export const cycleOracleOverride = (id: string) => {
	const order: OracleOverride[] = ["auto", "veto", "approve"];
	const current = gateLabStore.state.overrides[id] ?? "auto";
	const next = order[(order.indexOf(current) + 1) % order.length];
	setOracleOverride(id, next);
};

export const setRiskField = <K extends keyof RiskDraft>(
	key: K,
	value: number,
) => {
	if (!Number.isFinite(value)) return;

	gateLabStore.setState((prev) => ({
		...prev,
		risk: { ...prev.risk, [key]: value },
	}));
};

export const setResonanceField = <K extends keyof ResonanceDraft>(
	key: K,
	value: ResonanceDraft[K],
) => {
	gateLabStore.setState((prev) => ({
		...prev,
		resonance: { ...prev.resonance, [key]: value },
	}));
};

export const toggleResonanceBypass = () => {
	gateLabStore.setState((prev) => ({
		...prev,
		resonance: { ...prev.resonance, bypass: !prev.resonance.bypass },
	}));
};

export const resetGateLab = () => {
	gateLabStore.setState((prev) => ({
		...prev,
		overrides: emptyOverrides(),
		risk: { ...RISK_DEFAULTS },
		resonance: { ...RESONANCE_DEFAULTS },
	}));
};
