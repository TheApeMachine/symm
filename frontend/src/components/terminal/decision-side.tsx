import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { causalStore } from "#/collections/causal";
import {
	type CognitiveReading,
	cognitiveScopes,
	cognitiveStore,
} from "#/collections/cognitive";

type Rung = {
	rung: number;
	name: string;
	desc: string;
	key: string;
	color: string;
};

const RUNGS: Rung[] = [
	{
		rung: 1,
		name: "Association",
		desc: "P(y | x)",
		key: "beta",
		color: "var(--info)",
	},
	{
		rung: 2,
		name: "Intervention",
		desc: "P(y | do(x))",
		key: "intervention",
		color: "var(--acc)",
	},
	{
		rung: 3,
		name: "Counterfactual",
		desc: "Pearl strength",
		key: "strength",
		color: "var(--up)",
	},
];

const isConcreteSymbol = (symbol: string | undefined): symbol is string =>
	symbol !== undefined && symbol !== "" && symbol !== "stream";

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

export const CausalLadder = ({ symbol }: { symbol?: string }) => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const scope = isConcreteSymbol(symbol) ? symbol : focusSymbol;
	const frame = useSelector(
		causalStore,
		(state) => state.causal[scope]?.values().at(-1),
	);

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
				pearl do-calculus · {String(frame.category)}
			</div>

			<div className="flex flex-col gap-2.5">
				{RUNGS.map((rung) => {
					const raw = Number(frame[rung.key] ?? 0);
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
			<div className="mt-3 grid grid-cols-2 gap-1.5 border-(--line) border-t pt-2 font-mono text-[10px]">
				<div className="flex justify-between">
					<span className="text-(--f4)">uplift</span>
					<span className="text-(--f1)">
						{Number(frame.uplift ?? 0).toFixed(3)}
					</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f4)">residual</span>
					<span className="text-(--f1)">
						{Number(frame.residual ?? 0).toFixed(3)}
					</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f4)">baseline</span>
					<span className="text-(--f1)">
						{Number(frame.baseline ?? 0).toFixed(3)}
					</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f4)">panic</span>
					<span className="text-(--f1)">
						{Number(frame.panic ?? 0).toFixed(3)}
					</span>
				</div>
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
