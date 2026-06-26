import { useSelector } from "@tanstack/react-store";
import { measurementsStore } from "#/collections/measurements";
import { playbookStore } from "#/collections/playbook";
import { tickStore } from "#/collections/tick";
import { CausalLadder } from "#/components/terminal/decision-side";
import { entryLineStats, fixed } from "#/components/terminal/decision-format";
import { terminalDecisionsFromWalk } from "#/components/terminal/decisions-from-walk";
import { kernelsForFocus } from "#/components/terminal/kernels";

const verdictStyle = (
	verdict: "allow" | "in-play" | "blocked",
): { bg: string; fg: string } => {
	if (verdict === "allow") {
		return {
			bg: "color-mix(in srgb, var(--up) 16%, transparent)",
			fg: "var(--up)",
		};
	}

	if (verdict === "in-play") {
		return {
			bg: "color-mix(in srgb, var(--info) 16%, transparent)",
			fg: "var(--info)",
		};
	}

	return { bg: "var(--line)", fg: "var(--f3)" };
};

const FunnelCard = ({
	label,
	value,
	sub,
	accent = false,
}: {
	label: string;
	value: string | number;
	sub: string;
	accent?: boolean;
}) => (
	<div className="relative flex-1 rounded border border-(--line) bg-(--surface) px-3 py-2.5">
		<div className="text-[9.5px] text-(--f4) uppercase tracking-widest">
			{label}
		</div>
		<div
			className="mt-0.5 font-mono font-semibold text-[26px] leading-[1.1]"
			style={{ color: accent ? "var(--acc)" : "var(--f1)" }}
		>
			{value}
		</div>
		<div className="mt-px font-mono text-[9.5px] text-(--f4)">{sub}</div>
	</div>
);

/*
DecisionTreeView renders the single decision point: the measure→candidate→chosen
funnel, the derived entry line, and one row per evaluated symbol with its
per-source attribution and playbook verdict. Every value comes from the live
backend stream (tick counts, walk traces, measurement kernels) — nothing here is
simulated. The right rail shows the causal ladder for the leading candidate.
*/
export const DecisionTreeView = () => {
	const evaluations = useSelector(playbookStore, (state) => state.evaluations);
	const readings = useSelector(measurementsStore, (state) => state);
	const tick = useSelector(tickStore, (state) => state.frame);

	const kernels = kernelsForFocus(readings);
	const rows = terminalDecisionsFromWalk(evaluations, kernels);
	const entry = entryLineStats(rows.map((row) => row.scoreValue));

	const measurements = Number(tick?.measurements ?? 0);
	const candidates = Number(tick?.candidates ?? 0);
	const chosen = Number(tick?.chosen ?? 0);
	const open = Number(tick?.open ?? 0);

	const leader = rows[0]?.symbol;

	if (rows.length === 0) {
		return (
			<div className="grid h-full min-w-[1040px] place-items-center font-mono text-(--f4) text-sm">
				waiting for playbook walk frames
			</div>
		);
	}

	return (
		<div className="grid h-full min-h-0 min-w-[1040px] grid-cols-[minmax(640px,1fr)_332px]">
			<div className="min-h-0 overflow-auto px-5 py-4">
				<div className="mb-4 flex items-stretch gap-2.5">
					<FunnelCard
						label="Measured"
						value={measurements}
						sub="signals this tick"
					/>
					<FunnelCard
						label="Candidates"
						value={candidates}
						sub="playbook proposed"
					/>
					<FunnelCard label="Chosen" value={chosen} sub="admitted" accent />
					<FunnelCard label="Open" value={open} sub="positions held" />
				</div>

				<div className="mb-3.5 flex items-center gap-3.5 rounded border border-(--line) bg-(--sunken) px-3 py-2 font-mono text-[11.5px]">
					<span className="text-(--f3)">entry line</span>
					<span className="font-semibold text-(--acc)">
						{fixed(entry.line)}
					</span>
					<span className="text-(--f4)">·</span>
					<span className="text-(--f3)">median {fixed(entry.median)}</span>
					<span className="text-(--f4)">·</span>
					<span className="text-(--f3)">mad {fixed(entry.mad)}</span>
					<span className="ml-auto text-(--f4)">
						support gate ≥ 2 · edge ≥ req
					</span>
				</div>

				<div className="mb-2 flex items-baseline justify-between">
					<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Candidate evaluation
					</span>
					<span className="font-mono text-[9.5px] text-(--f4)">
						{rows.length} symbols walked
					</span>
				</div>

				<div className="flex flex-col gap-1.5">
					{rows.map((row) => {
						const style = verdictStyle(row.verdict);
						const scorePct = Math.min(100, row.scoreValue * 100);

						return (
							<div
								key={row.key}
								className="overflow-hidden rounded border border-(--line) bg-(--surface)"
							>
								<div className="grid grid-cols-[78px_1fr_132px_92px] items-center gap-3 px-3 py-2.5">
									<div>
										<div className="font-mono font-semibold text-[13px] text-(--f1)">
											{row.symbol}
										</div>
										<div className="font-mono text-[9px] text-(--f4)">
											×{row.signals.length} src
										</div>
									</div>

									<div className="flex flex-col gap-1">
										{row.signals.slice(0, 3).map((signal) => (
											<div
												key={signal.source}
												className="flex items-center gap-1.5"
											>
												<span className="w-16 font-mono text-[9px] text-(--f4)">
													{signal.source}
												</span>
												<div className="h-1 flex-1 overflow-hidden rounded-sm bg-(--line)">
													<div
														className="h-full bg-(--info)"
														style={{ width: `${signal.confidence * 100}%` }}
													/>
												</div>
												<span className="w-[30px] text-right font-mono text-[9px] text-(--f3)">
													{Math.round(signal.confidence * 100)}
												</span>
											</div>
										))}
									</div>

									<div>
										<div className="mb-0.5 flex items-center justify-between font-mono text-[9.5px] text-(--f4)">
											<span>combined</span>
											<span className="text-(--f1)">{row.scoreText}</span>
										</div>
										<div className="relative h-1.5 overflow-hidden rounded-sm bg-(--line)">
											<div
												className="h-full"
												style={{
													width: `${scorePct}%`,
													background: row.edgePositive
														? "var(--up)"
														: "var(--down)",
												}}
											/>
										</div>
										<div
											className="mt-1.5 font-mono text-[9px]"
											style={{
												color: row.edgePositive ? "var(--up)" : "var(--f4)",
											}}
										>
											edge {row.edgeText}
										</div>
									</div>

									<div className="text-right">
										<span
											className="inline-block rounded-sm px-2.5 py-1 font-semibold text-[10px] uppercase"
											style={{ background: style.bg, color: style.fg }}
										>
											{row.verdict}
										</span>
										<div className="mt-1.5 font-mono text-[9px] text-(--f4)">
											{row.why}
										</div>
									</div>
								</div>
							</div>
						);
					})}
				</div>
			</div>

			<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
				<CausalLadder symbol={leader} />
			</div>
		</div>
	);
};
