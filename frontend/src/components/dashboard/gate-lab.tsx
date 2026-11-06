import { useSelector } from "@tanstack/react-store";
import {
	cycleOracleOverride,
	gateLabStore,
	ORACLE_GATES,
	type ResonanceDraft,
	type RiskDraft,
	resetGateLab,
	setResonanceField,
	setRiskField,
	toggleGateLab,
	toggleResonanceBypass,
} from "#/collections/gate-lab";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Input } from "#/components/ui/input";
import { Typography } from "#/components/ui/typography";
import { trySendCommand } from "#/lib/hub-socket";
import { cn } from "#/lib/utils";

const RISK_FIELDS: Array<{
	key: keyof RiskDraft;
	label: string;
	step: string;
	hint: string;
}> = [
	{
		key: "min_mover_volume_usd",
		label: "vol$",
		step: "1000",
		hint: "Minimum 24h quote volume (USD). Below this, Volume quote vetoes the name.",
	},
	{
		key: "min_mover_change_pct",
		label: "day%",
		step: "0.1",
		hint: "Minimum open-to-last move (%). Flat names fail Day move even if volume looks fine.",
	},
	{
		key: "max_entry_spread_pct",
		label: "spr%",
		step: "0.1",
		hint: "Maximum touch spread (%) allowed at entry. Wider than this → Spread veto.",
	},
	{
		key: "max_entry_peak_frac",
		label: "peak",
		step: "0.05",
		hint: "Max last position in the 24h high–low range (0–1). At/above → 24h peak veto. 0.90 = top 10% blocked. ≤0 or ≥1 disables.",
	},
	{
		key: "min_touch_bid_cash",
		label: "cash",
		step: "10",
		hint: "Minimum top-of-book bid notional. Thin touch cash fails Book integrity.",
	},
	{
		key: "min_backing_ratio",
		label: "back",
		step: "0.05",
		hint: "How much bid/ask support must sit inside the mid band (0–1). Low backing = fake wall.",
	},
	{
		key: "min_tape_notional_usd",
		label: "tape$",
		step: "10",
		hint: "Quote dollars in the last tape window must beat the prior window and clear this floor. A fading tape fails Book integrity.",
	},
	{
		key: "max_entry_open_drag_pct",
		label: "drag%",
		step: "0.1",
		hint: "Max close-now drag (%) you’ll accept on entry. High drag → Capacity / integrity veto.",
	},
];

const RESONANCE_FIELDS: Array<{
	key: Exclude<keyof ResonanceDraft, "bypass">;
	label: string;
	step: string;
	hint: string;
}> = [
	{
		key: "min_task_relative_precision",
		label: "prec",
		step: "0.05",
		hint: "Minimum task-relative precision. Below this the model skips with the resonance gate.",
	},
	{
		key: "min_surprise",
		label: "surp",
		step: "0.05",
		hint: "Minimum surprise. Low surprise = manifold thinks nothing moved → skip.",
	},
	{
		key: "min_supported_horizon",
		label: "horz",
		step: "1",
		hint: "Minimum supported forward-curve horizon (steps). Short horizon → skip.",
	},
	{
		key: "min_enter_z",
		label: "enterZ",
		step: "0.05",
		hint: "Minimum forward-story enter/hold net (z). Below this → story skip. Bypass forces bullish.",
	},
];

const overrideTone = (value: string) => {
	if (value === "approve") return "border-(--up)/50 text-(--up)";
	if (value === "veto") return "border-(--down)/50 text-(--down)";
	return "border-(--line) text-(--f3)";
};

const Hint = ({ text }: { text: string }) => (
	<span
		role="tooltip"
		className={cn(
			"pointer-events-none absolute top-full left-0 z-20 mt-1 w-52 max-w-[70vw]",
			"border border-(--line) bg-(--surface) px-2 py-1.5 font-mono text-[9px] leading-snug text-(--f2)",
			"opacity-0 shadow-sm transition-opacity duration-100",
			"group-hover:opacity-100 group-focus-within:opacity-100",
		)}
	>
		{text}
	</span>
);

export const GateLabToggle = () => {
	const open = useSelector(gateLabStore, (state) => state.open);

	return (
		<span className="group relative inline-flex">
			<Button
				variant="bare"
				size="xxs"
				onClick={toggleGateLab}
				data-gate-lab-toggle={open ? "open" : "closed"}
				className={cn(
					"rounded-xs border px-1.5 py-px text-[8.5px] uppercase tracking-[0.08em]",
					open
						? "border-(--acc)/50 text-(--acc)"
						: "border-(--line) text-(--f4)",
				)}
			>
				Lab
			</Button>
			<Hint text="Experiment panel: force oracle lamps and live-patch risk floors." />
		</span>
	);
};

export const GateLab = () => {
	const open = useSelector(gateLabStore, (state) => state.open);
	const overrides = useSelector(gateLabStore, (state) => state.overrides);
	const risk = useSelector(gateLabStore, (state) => state.risk);
	const resonance = useSelector(gateLabStore, (state) => state.resonance);

	if (!open) return null;

	const applyRisk = () => {
		trySendCommand({ type: "risk", ...risk });
	};

	const applyResonance = (draft = resonance) => {
		trySendCommand({ type: "resonance", ...draft });
	};

	return (
		<Flex.Column
			gap={2}
			padding={2}
			className="border-(--line) border-b bg-(--raised)"
			data-gate-lab="true"
		>
			<Flex.Row align="center" justify="between" gap={2}>
				<Typography.Span className="text-[9px] uppercase tracking-widest text-(--f3)">
					Gate lab · oracles · risk · resonance
				</Typography.Span>
				<Flex.Row gap={1}>
					<span className="group relative inline-flex">
						<Button
							variant="bare"
							size="xxs"
							onClick={resetGateLab}
							className="rounded-xs border border-(--line) px-1.5 py-px text-[8px] uppercase text-(--f4)"
						>
							Reset
						</Button>
						<Hint text="Clear forced oracles and restore default risk + resonance drafts." />
					</span>
					<span className="group relative inline-flex">
						<Button
							variant="bare"
							size="xxs"
							onClick={applyRisk}
							data-gate-lab-apply="true"
							className="rounded-xs border border-(--acc)/40 px-1.5 py-px text-[8px] uppercase text-(--acc)"
						>
							Apply risk
						</Button>
						<Hint text="Push risk floors into the live desk (WS risk). Max ceilings at 0 = unlimited. Oracle force lamps are local-only — use rez:bypass to skip the board." />
					</span>
					<span className="group relative inline-flex">
						<Button
							variant="bare"
							size="xxs"
							onClick={applyResonance}
							data-gate-lab-apply-resonance="true"
							className="rounded-xs border border-(--acc)/40 px-1.5 py-px text-[8px] uppercase text-(--acc)"
						>
							Apply rez
						</Button>
						<Hint text="Push resonance gate floors / bypass into the live model (WS resonance)." />
					</span>
				</Flex.Row>
			</Flex.Row>

			<Flex.Row gap={1} wrap="wrap" className="min-w-0">
				{ORACLE_GATES.map((gate) => {
					const value = overrides[gate.id] ?? "auto";
					const stateLine =
						value === "auto"
							? "Following live emit (brighter of veto/approve)."
							: value === "veto"
								? "Forced veto — lamp stays red."
								: "Forced approve — lamp stays green.";

					return (
						<span key={gate.id} className="group relative inline-flex">
							<Button
								variant="bare"
								size="xxs"
								onClick={() => cycleOracleOverride(gate.id)}
								data-gate-lab-oracle={gate.id}
								data-gate-lab-override={value}
								className={cn(
									"rounded-xs border px-1.5 py-px font-mono text-[8.5px] uppercase",
									overrideTone(value),
								)}
							>
								{gate.id}:{value}
							</Button>
							<Hint
								text={`${gate.label}. ${gate.hint} Click cycles auto → veto → approve. ${stateLine}`}
							/>
						</span>
					);
				})}
			</Flex.Row>

			<Flex.Row gap={2} wrap="wrap" className="min-w-0">
				{RISK_FIELDS.map((field) => {
					const inputId = `gate-lab-risk-${field.key}`;

					return (
						<label
							key={field.key}
							htmlFor={inputId}
							className="group relative flex min-w-18 flex-col gap-0.5"
						>
							<Typography.Span className="text-[8px] uppercase tracking-[0.06em] text-(--f4)">
								{field.label}
							</Typography.Span>
							<Input
								id={inputId}
								size="xxs"
								mono
								type="number"
								step={field.step}
								value={risk[field.key]}
								data-gate-lab-risk={field.key}
								onChange={(event) => {
									setRiskField(field.key, Number(event.target.value));
								}}
								className="border-(--line) border-b px-0.5"
							/>
							<Hint text={field.hint} />
						</label>
					);
				})}
			</Flex.Row>

			<Flex.Row gap={2} wrap="wrap" align="end" className="min-w-0">
				<span className="group relative inline-flex">
					<Button
						variant="bare"
						size="xxs"
						onClick={() => {
							toggleResonanceBypass();
							applyResonance({
								...gateLabStore.state.resonance,
								bypass: !resonance.bypass,
							});
						}}
						data-gate-lab-rez-bypass={resonance.bypass ? "on" : "off"}
						className={cn(
							"rounded-xs border px-1.5 py-px font-mono text-[8.5px] uppercase",
							resonance.bypass
								? "border-(--up)/50 text-(--up)"
								: "border-(--line) text-(--f3)",
						)}
					>
						rez:{resonance.bypass ? "bypass" : "gate"}
					</Button>
					<Hint text="Bypass auto-applies: skips missing readings, story floors, oracle board, and soft physical desk gates on virtual. Peak, day-move, and volume rails still block. Cold explores need a full-length path." />
				</span>
				{RESONANCE_FIELDS.map((field) => {
					const inputId = `gate-lab-rez-${field.key}`;

					return (
						<label
							key={field.key}
							htmlFor={inputId}
							className="group relative flex min-w-18 flex-col gap-0.5"
						>
							<Typography.Span className="text-[8px] uppercase tracking-[0.06em] text-(--f4)">
								{field.label}
							</Typography.Span>
							<Input
								id={inputId}
								size="xxs"
								mono
								type="number"
								step={field.step}
								value={resonance[field.key]}
								data-gate-lab-rez={field.key}
								onChange={(event) => {
									setResonanceField(field.key, Number(event.target.value));
								}}
								className="border-(--line) border-b px-0.5"
							/>
							<Hint text={field.hint} />
						</label>
					);
				})}
			</Flex.Row>
		</Flex.Column>
	);
};
