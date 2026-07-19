import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
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
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { getWorker } from "#/providers/websocket";
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

type CausalLadderRefs = {
	waiting: HTMLDivElement | null;
	panel: HTMLDivElement | null;
	subtitle: HTMLSpanElement | null;
	rungValues: Array<HTMLSpanElement | null>;
	rungFills: Array<HTMLDivElement | null>;
	footerValues: Record<
		(typeof FOOTER_FIELDS)[number]["key"],
		HTMLSpanElement | null
	>;
};

const rungToneClass = (variant: Variant): string => {
	if (variant === "success") {
		return "font-mono text-[11px] text-(--up)";
	}

	if (variant === "warning") {
		return "font-mono text-[11px] text-(--acc)";
	}

	return "font-mono text-[11px] text-(--info)";
};

const paintCausalLadder = (
	refs: CausalLadderRefs,
	frame: CausalFrame | undefined,
): void => {
	const waiting = frame === undefined;

	if (refs.waiting !== null) {
		refs.waiting.style.display = waiting ? "" : "none";
	}

	if (refs.panel !== null) {
		refs.panel.style.display = waiting ? "none" : "";
	}

	if (waiting) {
		return;
	}

	if (refs.subtitle !== null) {
		refs.subtitle.textContent = `pearl do-calculus · ${String(causalCategory(frame))}`;
	}

	for (const [index, rung] of RUNGS.entries()) {
		const value = causalUnit(rung.value(frame));

		if (refs.rungValues[index] !== null) {
			refs.rungValues[index].textContent = value.toFixed(3);
			refs.rungValues[index].className = rungToneClass(rung.variant);
		}

		if (refs.rungFills[index] !== null) {
			refs.rungFills[index].style.width = `${value * 100}%`;
			refs.rungFills[index].style.background =
				rung.variant === "success"
					? "var(--success)"
					: rung.variant === "warning"
						? "var(--warning)"
						: "var(--info)";
		}
	}

	for (const field of FOOTER_FIELDS) {
		const node = refs.footerValues[field.key];

		if (node !== null) {
			node.textContent = field.value(frame).toFixed(3);
		}
	}
};

const isConcreteSymbol = (symbol: string | undefined): symbol is string =>
	symbol !== undefined && symbol !== "" && symbol !== "stream";

/*
CausalLadder paints Pearl ladder readings from causalStore without React
reconciliation on each websocket tick.
*/
export const CausalLadder = ({ symbol }: { symbol?: string }) => {
	const waitingRef = useRef<HTMLDivElement>(null);
	const panelRef = useRef<HTMLDivElement>(null);
	const subtitleRef = useRef<HTMLSpanElement>(null);
	const rungValueRefs = useRef<Array<HTMLSpanElement | null>>([]);
	const rungFillRefs = useRef<Array<HTMLDivElement | null>>([]);
	const upliftRef = useRef<HTMLSpanElement>(null);
	const residualRef = useRef<HTMLSpanElement>(null);
	const baselineRef = useRef<HTMLSpanElement>(null);
	const panicRef = useRef<HTMLSpanElement>(null);

	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const online = useSelector(appStore, (state) => state.online);
	const scope = isConcreteSymbol(symbol) ? symbol : focusSymbol;

	useDirectStorePaint(
		getWorker(),
		[{ store: "causal", key: scope }],
		(buffers) =>
			paintCausalLadder(
				{
					waiting: waitingRef.current,
					panel: panelRef.current,
					subtitle: subtitleRef.current,
					rungValues: rungValueRefs.current,
					rungFills: rungFillRefs.current,
					footerValues: {
						uplift: upliftRef.current,
						residual: residualRef.current,
						baseline: baselineRef.current,
						panic: panicRef.current,
					},
				},
				latestCausalFrame(
					(buffers[`causal:${scope}`] ?? []) as CausalFrame[],
				),
			),
		[online, symbol, scope],
	);

	return (
		<Panel>
			<div ref={waitingRef}>
				<div className="font-semibold text-[12px] text-(--f1)">
					Causal ladder
				</div>
				<div className="mt-2 font-mono text-[9.5px] text-(--f4)">
					no causal reading yet
				</div>
			</div>

			<div ref={panelRef} style={{ display: "none" }}>
				<div className="font-semibold text-[12px] text-(--f1)">
					Causal ladder
				</div>
				<div className="mt-0.5 mb-3 font-mono text-[9.5px] text-(--f4)">
					<span ref={subtitleRef} />
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
								<span
									ref={(node) => {
										rungValueRefs.current[index] = node;
									}}
								/>
							</div>
							<div className="my-1.5 font-mono text-[9px] text-(--f4)">
								{rung.desc}
							</div>
							<div className="h-[5px] overflow-hidden rounded-[3px] bg-(--line)">
								<div
									ref={(node) => {
										rungFillRefs.current[index] = node;
									}}
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
										? upliftRef
										: field.key === "residual"
											? residualRef
											: field.key === "baseline"
												? baselineRef
												: panicRef
								}
								className="text-(--f1)"
							/>
						</div>
					))}
				</div>
			</div>
		</Panel>
	);
};

type DecisionsEntryLineRefs = {
	panel: HTMLDivElement | null;
	entryLine: HTMLSpanElement | null;
	strength: HTMLSpanElement | null;
	confidence: HTMLSpanElement | null;
};

function paintDecisionsEntryLine(
	refs: DecisionsEntryLineRefs,
	symbol: string | undefined,
	rows: CausalFrame[] | undefined,
): void {
	const visible = symbol !== undefined;

	if (refs.panel !== null) {
		refs.panel.style.display = visible ? "" : "none";
	}

	if (!visible) {
		return;
	}

	const frame = latestCausalFrame(rows);
	const entryLine = causalEntryBaseline(frame);
	const entryScore = causalStrength(frame);
	const entryConfidence = causalConfidence(frame);

	if (refs.entryLine !== null) {
		refs.entryLine.textContent = fixed(entryLine);
	}

	if (refs.strength !== null) {
		refs.strength.textContent = fixed(entryScore);
	}

	if (refs.confidence !== null) {
		refs.confidence.textContent = fixed(entryConfidence);
	}
}

// LiveDecisionsEntryLine paints the selected candidate entry gate without React
// reconciliation when causal frames advance.
export const LiveDecisionsEntryLine = ({
	symbol,
}: {
	symbol: string | undefined;
}) => {
	const panelRef = useRef<HTMLDivElement>(null);
	const entryLineRef = useRef<HTMLSpanElement>(null);
	const strengthRef = useRef<HTMLSpanElement>(null);
	const confidenceRef = useRef<HTMLSpanElement>(null);

	const online = useSelector(appStore, (state) => state.online);

	useDirectStorePaint(
		getWorker(),
		[{ store: "causal", key: symbol ?? "" }],
		(buffers) =>
			paintDecisionsEntryLine(
				{
					panel: panelRef.current,
					entryLine: entryLineRef.current,
					strength: strengthRef.current,
					confidence: confidenceRef.current,
				},
				symbol,
				(buffers[`causal:${symbol ?? ""}`] ?? []) as CausalFrame[],
			),
		[online, symbol],
	);

	return (
		<div ref={panelRef} style={{ display: "none" }}>
			<Panel className="mb-3.5 flex items-center gap-3.5 px-3 py-2 font-mono text-[11.5px]">
				<span className="text-(--f3)">entry line</span>
				<span ref={entryLineRef} className="font-semibold text-(--acc)" />
				<span className="text-(--f4)">·</span>
				<span className="text-(--f3)">
					strength <span ref={strengthRef} />
				</span>
				<span className="text-(--f4)">·</span>
				<span className="text-(--f3)">
					confidence <span ref={confidenceRef} />
				</span>
				<span className="ml-auto text-(--f4)">
					support gate ≥ 2 · strategy utility wins
				</span>
			</Panel>
		</div>
	);
};

export const DecisionSideRail = ({ symbol }: { symbol?: string }) => (
	<>
		<CausalLadder symbol={symbol} />
		<CognitiveBeam symbol={symbol} />
	</>
);
