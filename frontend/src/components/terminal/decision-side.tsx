import { useSelector } from "@tanstack/react-store";
import { measurementsStore } from "#/collections/measurements";

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

const readingFor = (
	readings: Record<string, Record<string, unknown>>,
	origin: string,
	symbol: string | undefined,
): Record<string, unknown> | undefined => {
	const bySymbol = readings[origin] as
		| Record<string, Record<string, unknown>>
		| undefined;

	if (bySymbol === undefined) {
		return undefined;
	}

	if (symbol !== undefined && bySymbol[symbol] !== undefined) {
		return bySymbol[symbol];
	}

	const first = Object.keys(bySymbol)[0];

	return first === undefined ? undefined : bySymbol[first];
};

/*
CausalLadder renders the three rungs of Pearl's do-calculus from the live causal
measurement for the leading candidate. Each rung's fill is the real mass the
causal signal published (beta, uplift, alpha) — no fabricated values; an absent
causal reading renders an explicit empty state.
*/
export const CausalLadder = ({ symbol }: { symbol?: string }) => {
	const readings = useSelector(measurementsStore, (state) => state);
	const frame = readingFor(readings, "causal", symbol);
	const output = (frame?.output ?? {}) as Record<string, number>;

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
				pearl do-calculus · {symbol ?? "leading candidate"}
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

export const DecisionSideRail = CausalLadder;
