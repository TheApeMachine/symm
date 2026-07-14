import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
import { appStore } from "#/collections/app";
import { causalStore } from "#/collections/causal";
import {
	type CognitiveReading,
	cognitiveScopes,
	cognitiveStore,
} from "#/collections/cognitive";
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
import { fixed } from "#/components/terminal/decision-format";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { Meter } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import type { Variant } from "@/components/ui/types";

type Rung = {
	rung: number;
	name: string;
	desc: string;
	value: (frame: ReturnType<typeof latestCausalFrame>) => number;
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

const paintCausalLadder = (refs: CausalLadderRefs, scope: string): void => {
	const frame = latestCausalFrame(scope);
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

export type CognitiveBeamModel = {
	cohort: string;
	sequence: string;
	winner: string;
	paths: string;
	meters: Array<{
		label: string;
		value: string;
		percent: number;
		variant: Variant;
	}>;
};

export const cognitiveReadingFor = (
	readings: Record<string, CognitiveReading>,
	symbol?: string,
): CognitiveReading | null => {
	if (isConcreteSymbol(symbol)) {
		return readings[symbol] ?? null;
	}

	const [scope] = cognitiveScopes(readings);

	return scope === undefined ? null : readings[scope];
};

export const cognitiveBeamModel = (
	reading: CognitiveReading | null,
): CognitiveBeamModel | null => {
	if (reading === null) {
		return null;
	}

	const entropyBits =
		typeof reading.entropyBits === "number" &&
		Number.isFinite(reading.entropyBits)
			? reading.entropyBits
			: 0;
	const entropyThreshold =
		typeof reading.entropyThreshold === "number" &&
		Number.isFinite(reading.entropyThreshold)
			? reading.entropyThreshold
			: 0;
	const confidence = Math.min(
		1,
		Math.max(
			0,
			typeof reading.classConfidence === "number" &&
				Number.isFinite(reading.classConfidence)
				? reading.classConfidence
				: 0,
		),
	);
	const lookahead = Math.min(
		1,
		Math.max(
			0,
			typeof reading.lookaheadScore === "number" &&
				Number.isFinite(reading.lookaheadScore)
				? reading.lookaheadScore
				: 0,
		),
	);
	const paths =
		(typeof reading.lookaheadPaths === "number" &&
		Number.isFinite(reading.lookaheadPaths)
			? reading.lookaheadPaths
			: null) ??
		(typeof reading.prewarmPaths === "number" &&
		Number.isFinite(reading.prewarmPaths)
			? reading.prewarmPaths
			: 0);
	const entropyPercent =
		entropyThreshold > 0
			? Math.min(100, Math.max(0, (entropyBits / entropyThreshold) * 100))
			: 0;

	return {
		cohort: String(reading.regimeCohort),
		sequence: reading.sequence || "waiting",
		winner: reading.winnerClass || "pending",
		paths: String(Math.round(paths)),
		meters: [
			{
				label: "Entropy gate",
				value: `${entropyBits.toFixed(2)} / ${entropyThreshold.toFixed(1)} bits`,
				percent: entropyPercent,
				variant: "success",
			},
			{
				label: "Class confidence",
				value: `${Math.round(confidence * 100)}%`,
				percent: confidence * 100,
				variant: "info",
			},
			{
				label: "Lookahead beam",
				value: lookahead.toFixed(3),
				percent: lookahead * 100,
				variant: "warning",
			},
		],
	};
};

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

	useDirectStorePaint(
		() =>
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
				isConcreteSymbol(symbol) ? symbol : appStore.state.focusSymbol,
			),
		[causalStore, appStore],
		[symbol],
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
): void {
	const visible = symbol !== undefined;

	if (refs.panel !== null) {
		refs.panel.style.display = visible ? "" : "none";
	}

	if (!visible) {
		return;
	}

	const frame = latestCausalFrame(symbol ?? "");
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

	useDirectStorePaint(
		() =>
			paintDecisionsEntryLine(
				{
					panel: panelRef.current,
					entryLine: entryLineRef.current,
					strength: strengthRef.current,
					confidence: confidenceRef.current,
				},
				symbol,
			),
		[causalStore, appStore],
		[symbol],
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

export const CognitiveBeam = ({ symbol }: { symbol?: string }) => {
	const readings = useSelector(cognitiveStore, (state) => state.readings);
	const model = cognitiveBeamModel(cognitiveReadingFor(readings, symbol));

	if (model === null) {
		return (
			<Panel className="mt-3.5">
				<div className="font-semibold text-[12px] text-(--f1)">
					Cognitive beam
				</div>
				<div className="mt-2 font-mono text-[9.5px] text-(--f4)">
					waiting for cognitive frame
				</div>
			</Panel>
		);
	}

	return (
		<Panel className="mt-3.5">
			<div className="flex items-center justify-between">
				<span className="font-semibold text-[12px] text-(--f1)">
					Cognitive beam
				</span>
				<span className="rounded-full border border-(--line2) px-2 py-px font-mono text-[9px] text-(--info)">
					cohort {model.cohort}
				</span>
			</div>
			<div className="mt-2 font-mono text-[9.5px] text-(--f4)">
				DMT sequence
			</div>
			<div className="mt-1 break-all rounded-sm border border-(--line) bg-(--bg) p-1.5 font-mono text-[10px] text-(--f2)">
				{model.sequence}
			</div>

			<div className="mt-3 flex flex-col gap-2.5">
				{model.meters.map((meter) => (
					<Meter
						key={meter.label}
						layout="stacked"
						label={meter.label}
						value={meter.value}
						percent={meter.percent}
						variant={meter.variant}
						size="s"
					/>
				))}
			</div>

			<div className="mt-3 grid grid-cols-2 gap-1.5 font-mono text-[10px]">
				<div className="flex justify-between">
					<span className="text-(--f4)">winner</span>
					<span className="text-(--acc)">{model.winner}</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f4)">paths</span>
					<span className="text-(--f1)">{model.paths}</span>
				</div>
			</div>
		</Panel>
	);
};

export const DecisionSideRail = ({ symbol }: { symbol?: string }) => (
	<>
		<CausalLadder symbol={symbol} />
		<CognitiveBeam symbol={symbol} />
	</>
);
