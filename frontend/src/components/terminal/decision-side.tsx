import { useSelector } from "@tanstack/react-store";
import {
	type CognitiveReading,
	cognitiveScopes,
	cognitiveStore,
} from "#/collections/cognitive";
import {
	type MeasurementsState,
	measurementsStore,
} from "#/collections/measurements";

type Rung = {
	rung: number;
	name: string;
	desc: string;
	key: string;
	color: string;
};

// Pearl's ladder, mapped onto the causal signal's real output masses. The causal
// measurement decomposes each move into endogenous alpha (the do(flow)
// counterfactual), systemic beta (intervention/shared drift), liquidity shock,
// and unexplained noise — exactly the association → intervention → counterfactual
// climb the panel narrates.
const RUNGS: Rung[] = [
	{
		rung: 1,
		name: "Association",
		desc: "P(y | x) · shared drift (beta)",
		key: "beta",
		color: "var(--info)",
	},
	{
		rung: 2,
		name: "Intervention",
		desc: "P(y | do(x)) · uplift from acting",
		key: "uplift",
		color: "var(--acc)",
	},
	{
		rung: 3,
		name: "Counterfactual",
		desc: "endogenous alpha vs noise",
		key: "alpha",
		color: "var(--up)",
	},
];

const isConcreteSymbol = (symbol: string | undefined): symbol is string =>
	symbol !== undefined && symbol !== "" && symbol !== "stream";

export const causalReadingFor = (
	readings: MeasurementsState,
	origin: string,
	symbol: string | undefined,
): Record<string, unknown> | undefined => {
	if (isConcreteSymbol(symbol)) {
		for (
			let index = (readings.symbols[symbol] ?? []).length - 1;
			index >= 0;
			index -= 1
		) {
			const frame = readings.symbols[symbol]?.[index];

			if (frame?.source === origin) {
				return frame;
			}
		}

		return undefined;
	}

	return readings.measurements[origin]?.values().at(-1);
};

const clamp = (value: number, min: number, max: number): number =>
	Math.min(max, Math.max(min, value));

const finite = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

export type CognitiveBeamModel = {
	cohort: string;
	sequence: string;
	winner: string;
	paths: string;
	meters: Array<{
		label: string;
		value: string;
		percent: number;
		color: string;
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

	const entropyBits = finite(reading.entropyBits) ?? 0;
	const entropyThreshold = finite(reading.entropyThreshold) ?? 0;
	const confidence = clamp(finite(reading.classConfidence) ?? 0, 0, 1);
	const lookahead = clamp(finite(reading.lookaheadScore) ?? 0, 0, 1);
	const paths =
		finite(reading.lookaheadPaths) ?? finite(reading.prewarmPaths) ?? 0;
	const entropyPercent =
		entropyThreshold > 0
			? clamp((entropyBits / entropyThreshold) * 100, 0, 100)
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
				color: "var(--up)",
			},
			{
				label: "Class confidence",
				value: `${Math.round(confidence * 100)}%`,
				percent: confidence * 100,
				color: "var(--info)",
			},
			{
				label: "Lookahead beam",
				value: lookahead.toFixed(3),
				percent: lookahead * 100,
				color: "var(--acc)",
			},
		],
	};
};

/*
CausalLadder renders the three rungs of Pearl's do-calculus from the live causal
measurement for the leading candidate. Each rung's fill is the real mass the
causal signal published (beta, uplift, alpha) — no fabricated values; an absent
causal reading renders an explicit empty state.
*/
export const CausalLadder = ({ symbol }: { symbol?: string }) => {
	const readings = useSelector(measurementsStore, (state) => state);
	const cognitiveReadings = useSelector(
		cognitiveStore,
		(state) => state.readings,
	);
	const frame = causalReadingFor(readings, "causal", symbol);
	const output = {
		...(frame ?? {}),
		...(frame?.metrics as Record<string, unknown> | undefined),
		...(frame?.output as Record<string, unknown> | undefined),
	} as Record<string, number>;
	const cognitive = cognitiveReadingFor(cognitiveReadings, symbol);
	const subtitle =
		cognitive?.regimePrefix && cognitive.regimePrefix !== ""
			? cognitive.regimePrefix
			: (symbol ?? "leading candidate");

	if (frame === undefined) {
		return (
			<div className="rounded border border-(--line) bg-(--sunken) p-3">
				<div className="font-semibold text-[12px] text-(--f1)">
					Causal ladder
				</div>
				<div className="mt-2 font-mono text-[9.5px] text-(--f4)">
					no causal reading yet
				</div>
			</div>
		);
	}

	return (
		<div className="rounded border border-(--line) bg-(--sunken) p-3">
			<div className="font-semibold text-[12px] text-(--f1)">Causal ladder</div>
			<div className="mt-0.5 mb-3 font-mono text-[9.5px] text-(--f4)">
				pearl do-calculus · {subtitle}
			</div>

			<div className="flex flex-col gap-2.5">
				{RUNGS.map((rung) => {
					const raw = Number(output[rung.key] ?? 0);
					const value = Math.max(0, Math.min(1, raw));

					return (
						<div
							key={rung.key}
							className="rounded-sm border border-(--line) bg-(--surface) px-2.5 py-2"
						>
							<div className="flex items-center justify-between">
								<span className="font-semibold text-[11.5px] text-(--f1)">
									{rung.rung}. {rung.name}
								</span>
								<span
									className="font-mono text-[11px]"
									style={{ color: rung.color }}
								>
									{value.toFixed(3)}
								</span>
							</div>
							<div className="my-1.5 font-mono text-[9px] text-(--f4)">
								{rung.desc}
							</div>
							<div className="h-[5px] overflow-hidden rounded-sm bg-(--line)">
								<div
									className="h-full transition-[width] duration-500"
									style={{ width: `${value * 100}%`, background: rung.color }}
								/>
							</div>
						</div>
					);
				})}
			</div>
		</div>
	);
};

export const CognitiveBeam = ({ symbol }: { symbol?: string }) => {
	const readings = useSelector(cognitiveStore, (state) => state.readings);
	const model = cognitiveBeamModel(cognitiveReadingFor(readings, symbol));

	if (model === null) {
		return (
			<div className="mt-3.5 rounded border border-(--line) bg-(--sunken) p-3">
				<div className="font-semibold text-[12px] text-(--f1)">
					Cognitive beam
				</div>
				<div className="mt-2 font-mono text-[9.5px] text-(--f4)">
					waiting for cognitive frame
				</div>
			</div>
		);
	}

	return (
		<div className="mt-3.5 rounded border border-(--line) bg-(--sunken) p-3">
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
					<div key={meter.label}>
						<div className="mb-1 flex justify-between text-[10.5px]">
							<span className="text-(--f3)">{meter.label}</span>
							<span className="font-mono text-(--f1)">{meter.value}</span>
						</div>
						<div className="h-[5px] overflow-hidden rounded-sm bg-(--line)">
							<div
								className="h-full"
								style={{
									width: `${meter.percent}%`,
									background: meter.color,
								}}
							/>
						</div>
					</div>
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
		</div>
	);
};

export const DecisionSideRail = ({ symbol }: { symbol?: string }) => (
	<>
		<CausalLadder symbol={symbol} />
		<CognitiveBeam symbol={symbol} />
	</>
);
