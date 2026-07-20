import { createRef } from "react";
import { appStore } from "#/collections/app";
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
import { CognitiveBeam } from "#/components/terminal/cognitive-beam";
import { fixed } from "#/components/terminal/decision-format";
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
		variant: "warning",
	},
	{
		rung: 3,
		name: "Counterfactual",
		desc: "Pearl strength",
		value: causalStrength,
		variant: "success",
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

const entryPanelRef = createRef<HTMLDivElement>();
const entryLineRef = createRef<HTMLSpanElement>();
const entryStrengthRef = createRef<HTMLSpanElement>();
const entryConfidenceRef = createRef<HTMLSpanElement>();

let ladderSymbol: string | undefined;
let entryLineSymbol: string | undefined;
let decisionsScopeSymbol: string | undefined;

/*
setDecisionsScopeSymbol pins the active candidate for decision-rail paints.
*/
export const setDecisionsScopeSymbol = (symbol: string | undefined): void => {
	decisionsScopeSymbol = symbol;
	entryLineSymbol = symbol;
	ladderSymbol = symbol;
};

/*
readDecisionsScopeSymbol returns the pinned candidate scope for DRAW paints.
*/
export const readDecisionsScopeSymbol = (): string | undefined =>
	decisionsScopeSymbol;

const rungToneClass = (variant: Variant): string => {
	if (variant === "success") {
		return "font-mono text-[11px] text-(--up)";
	}

	if (variant === "warning") {
		return "font-mono text-[11px] text-(--acc)";
	}

	return "font-mono text-[11px] text-(--info)";
};

const isConcreteSymbol = (symbol: string | undefined): symbol is string =>
	symbol !== undefined && symbol !== "" && symbol !== "stream";

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
	const scope = isConcreteSymbol(ladderSymbol)
		? ladderSymbol
		: isConcreteSymbol(decisionsScopeSymbol)
			? decisionsScopeSymbol
			: isConcreteSymbol(focusSymbol)
				? focusSymbol
				: appStore.state.focusSymbol;

	if (!isConcreteSymbol(scope)) {
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

const writeDecisionsEntryLine = (
	symbol: string | undefined,
	rows: CausalFrame[],
): void => {
	const visible = symbol !== undefined;

	if (entryPanelRef.current !== null) {
		entryPanelRef.current.style.display = visible ? "" : "none";
	}

	if (!visible) {
		return;
	}

	const frame = latestCausalFrame(rows, symbol);
	const entryLine = causalEntryBaseline(frame);
	const entryScore = causalStrength(frame);
	const entryConfidence = causalConfidence(frame);

	if (entryLineRef.current !== null) {
		entryLineRef.current.textContent = fixed(entryLine);
	}

	if (entryStrengthRef.current !== null) {
		entryStrengthRef.current.textContent = fixed(entryScore);
	}

	if (entryConfidenceRef.current !== null) {
		entryConfidenceRef.current.textContent = fixed(entryConfidence);
	}
};

/*
paintDecisionsEntryLine paints the selected candidate entry gate from the
current DRAW causal batch. Unmatched batches leave the prior gate values.
*/
export const paintDecisionsEntryLine = (
	value: unknown,
	_focusSymbol: string,
) => {
	const rows = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as CausalFrame[];

	if (entryLineSymbol === undefined && decisionsScopeSymbol === undefined) {
		writeDecisionsEntryLine(undefined, []);
		return;
	}

	const scope = isConcreteSymbol(entryLineSymbol)
		? entryLineSymbol
		: decisionsScopeSymbol;

	if (entryPanelRef.current !== null) {
		entryPanelRef.current.style.display = "";
	}

	for (let index = rows.length - 1; index >= 0; index -= 1) {
		const frame = rows[index];

		if (frame?.symbol === scope) {
			writeDecisionsEntryLine(scope, [frame]);
			return;
		}
	}
};

/*
LiveDecisionsEntryLine is the static entry-gate shell. DRAW paints via
paintDecisionsEntryLine.
*/
export const LiveDecisionsEntryLine = () => (
	<div ref={entryPanelRef} style={{ display: "none" }}>
			<Panel className="mb-3.5 flex items-center gap-3.5 px-3 py-2 font-mono text-[11.5px]">
				<span className="text-(--f3)">entry line</span>
				<span ref={entryLineRef} className="font-semibold text-(--acc)" />
				<span className="text-(--f4)">·</span>
				<span className="text-(--f3)">
					strength <span ref={entryStrengthRef} />
				</span>
				<span className="text-(--f4)">·</span>
				<span className="text-(--f3)">
					confidence <span ref={entryConfidenceRef} />
				</span>
				<span className="ml-auto text-(--f4)">
					support gate ≥ 2 · strategy utility wins
				</span>
		</Panel>
	</div>
);

export const DecisionSideRail = () => (
	<>
		<CausalLadder />
		<CognitiveBeam />
	</>
);
