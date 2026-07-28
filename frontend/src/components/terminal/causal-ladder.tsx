import { createRef } from "react";
import type { CausalFrame } from "#/collections/types";
import {
	causalAssociation,
	causalCategory,
	causalConfidence,
	causalContagion,
	causalEntryBaseline,
	causalIntervention,
	causalNoise,
	causalStrength,
	causalUnit,
	causalUplift,
	latestCausalFrame,
} from "#/components/terminal/causal-view";
import { readDecisionsScopeSymbol } from "#/components/terminal/decision-side";
import { Panel } from "@/components/ui/panel";
import type { Variant } from "@/components/ui/types";

type Rung = {
	rung: number;
	name: string;
	desc: string;
	value: (frame: CausalFrame | undefined) => number;
	variant: Variant;
};

const RUNGS: Rung[] = [
	{
		rung: 1,
		name: "Association",
		desc: "P(y | x)",
		value: causalAssociation,
		variant: "info",
	},
	{
		rung: 2,
		name: "Intervention",
		desc: "P(y | do(x))",
		value: causalIntervention,
		variant: "info",
	},
	{
		rung: 3,
		name: "Counterfactual",
		desc: "P(y' | x', x)",
		value: causalConfidence,
		variant: "warning",
	},
	{
		rung: 4,
		name: "Attribution",
		desc: "P(y | x, parents)",
		value: causalStrength,
		variant: "success",
	},
	{
		rung: 5,
		name: "Intervention uplift",
		desc: "P(y | do(x)) - P(y)",
		value: causalUplift,
		variant: "success",
	},
	{
		rung: 6,
		name: "Noise floor",
		desc: "P(y | ¬x)",
		value: causalNoise,
		variant: "disabled",
	},
];

const FOOTER_FIELDS = [
	{ key: "uplift", label: "uplift", value: causalUplift },
	{ key: "residual", label: "residual", value: causalNoise },
	{ key: "baseline", label: "baseline", value: causalEntryBaseline },
	{ key: "panic", label: "panic", value: causalContagion },
] as const;

const ladderWaitingRef = createRef<HTMLDivElement>();
const ladderPanelRef = createRef<HTMLDivElement>();
const ladderSubtitleRef = createRef<HTMLSpanElement>();
const ladderRungValueRefs = [
	createRef<HTMLSpanElement>(),
	createRef<HTMLSpanElement>(),
	createRef<HTMLSpanElement>(),
] as const;
const ladderRungFillRefs = [
	createRef<HTMLDivElement>(),
	createRef<HTMLDivElement>(),
	createRef<HTMLDivElement>(),
] as const;
const ladderUpliftRef = createRef<HTMLSpanElement>();
const ladderResidualRef = createRef<HTMLSpanElement>();
const ladderBaselineRef = createRef<HTMLSpanElement>();
const ladderPanicRef = createRef<HTMLSpanElement>();

const rungToneClass = (variant: Variant): string => {
	if (variant === "success") {
		return "font-mono text-[11px] text-(--up)";
	}

	if (variant === "warning") {
		return "font-mono text-[11px] text-(--acc)";
	}

	return "font-mono text-[11px] text-(--info)";
};

const writeCausalLadder = (frame: CausalFrame | undefined): void => {
	const waiting = frame === undefined;

	if (ladderWaitingRef.current !== null) {
		ladderWaitingRef.current.style.display = waiting ? "" : "none";
	}

	if (ladderPanelRef.current !== null) {
		ladderPanelRef.current.style.display = waiting ? "none" : "";
	}

	if (waiting) {
		return;
	}

	if (ladderSubtitleRef.current !== null) {
		ladderSubtitleRef.current.textContent = `pearl do-calculus · ${String(causalCategory(frame))}`;
	}

	for (const [index, rung] of RUNGS.entries()) {
		const value = causalUnit(rung.value(frame));
		const valueRef = ladderRungValueRefs[index]?.current;
		const fillRef = ladderRungFillRefs[index]?.current;

		if (valueRef !== null && valueRef !== undefined) {
			valueRef.textContent = value.toFixed(3);
			valueRef.className = rungToneClass(rung.variant);
		}

		if (fillRef !== null && fillRef !== undefined) {
			fillRef.style.width = `${value * 100}%`;
			fillRef.style.background =
				rung.variant === "success"
					? "var(--success)"
					: rung.variant === "warning"
						? "var(--warning)"
						: "var(--info)";
		}
	}

	const footerRefs = {
		uplift: ladderUpliftRef.current,
		residual: ladderResidualRef.current,
		baseline: ladderBaselineRef.current,
		panic: ladderPanicRef.current,
	} as const;

	for (const field of FOOTER_FIELDS) {
		const node = footerRefs[field.key];

		if (node !== null) {
			node.textContent = field.value(frame).toFixed(3);
		}
	}
};

/*
paintCausalLadder paints Pearl ladder readings from the current DRAW causal batch
into the static CausalLadder shell. Unmatched batches leave the prior paint.
*/
export const paintCausalLadder = (value: unknown, focusSymbol: string) => {
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as CausalFrame[];
	const scope = readDecisionsScopeSymbol() ?? focusSymbol;

	if (!scope) {
		writeCausalLadder(latestCausalFrame(rows));
		return;
	}

	for (let index = rows.length - 1; index >= 0; index -= 1) {
		const frame = rows[index];

		if (frame?.symbol === scope) {
			writeCausalLadder(frame);
			return;
		}
	}
};

/*
CausalLadder is the static Pearl ladder shell. DRAW paints via paintCausalLadder.
*/
export const CausalLadder = () => (
	<Panel>
		<div ref={ladderWaitingRef}>
			<div className="font-semibold text-[12px] text-(--f1)">
				Causal ladder
			</div>
			<div className="mt-2 font-mono text-[9.5px] text-(--f4)">
				no causal reading yet
			</div>
		</div>

		<div ref={ladderPanelRef} style={{ display: "none" }}>
			<div className="font-semibold text-[12px] text-(--f1)">
				Causal ladder
			</div>
			<div className="mt-0.5 mb-3 font-mono text-[9.5px] text-(--f4)">
				<span ref={ladderSubtitleRef} />
			</div>

			<div className="flex flex-col gap-2.5">
				{RUNGS.map((rung, index) => (
					<div
						key={rung.name}
						className="rounded-sm border border-(--line) bg-(--surface) px-2.5 py-2"
					>
						<div className="flex items-center justify-between">
							<span className="font-semibold text-[11.5px] text-(--f1)">
								{rung.rung}. {rung.name}
							</span>
							<span ref={ladderRungValueRefs[index]} />
						</div>
						<div className="my-1.5 font-mono text-[9px] text-(--f4)">
							{rung.desc}
						</div>
						<div className="h-[5px] overflow-hidden rounded-[3px] bg-(--line)">
							<div
								ref={ladderRungFillRefs[index]}
								className="h-full bg-(--info) transition-[width] duration-500 ease-out"
								style={{ width: "0%" }}
							/>
						</div>
					</div>
				))}
			</div>

			<div className="mt-3 grid grid-cols-2 gap-1.5 border-(--line) border-t pt-2 font-mono text-[10px]">
				{FOOTER_FIELDS.map((field) => (
					<div key={field.key} className="flex justify-between">
						<span className="text-(--f4)">{field.label}</span>
						<span
							ref={
								field.key === "uplift"
									? ladderUpliftRef
									: field.key === "residual"
										? ladderResidualRef
										: field.key === "baseline"
											? ladderBaselineRef
											: ladderPanicRef
							}
							className="text-(--f1)"
						/>
					</div>
				))}
			</div>
		</div>
	</Panel>
);
