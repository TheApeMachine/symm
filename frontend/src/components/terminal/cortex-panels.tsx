import { StatusBadge } from "#/components/dashboard/status-badge";
import {
	cognitiveBeamsFromReading,
	cognitivePosteriorFromReading,
} from "#/components/terminal/cognitive-viz";

const finite = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const clamp = (value: number, min: number, max: number): number =>
	Math.min(max, Math.max(min, value));

export const CortexBeamList = ({
	reading,
}: {
	reading: Record<string, unknown> | null;
}) => {
	const beams = cognitiveBeamsFromReading(reading);

	if (beams.length === 0) {
		return (
			<div className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)">
				waiting for cognitive beam reading
			</div>
		);
	}

	return (
		<div className="flex min-h-0 flex-1 flex-col gap-[5px] overflow-auto px-2 py-1.5">
			{beams.map((beam) => (
				<div
					key={beam.rank}
					className="flex items-center gap-2 rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-1.5"
				>
					<span
						className="w-4 shrink-0 font-mono text-[10px]"
						style={{ color: beam.color }}
					>
						#{beam.rank}
					</span>
					<span className="flex-1 font-mono text-[11px] text-(--f1)">
						{beam.sequence || "root"}
					</span>
					<div className="h-1 w-[70px] overflow-hidden rounded-[2px] bg-(--line)">
						<div
							className="h-full"
							style={{ width: `${beam.percent}%`, background: beam.color }}
						/>
					</div>
					<span className="w-11 shrink-0 text-right font-mono text-[9.5px] text-(--f3)">
						{beam.score}
					</span>
				</div>
			))}
		</div>
	);
};

export const CortexSidePanels = ({
	reading,
}: {
	reading: Record<string, unknown> | null;
}) => {
	const posterior = cognitivePosteriorFromReading(reading);
	const lookaheadScore = finite(reading?.lookaheadScore);
	const decay =
		lookaheadScore === null
			? "—"
			: Math.exp(Math.min(0, lookaheadScore)).toFixed(3);
	const replays = finite(reading?.lookaheadPaths);
	const entropyBits = finite(reading?.entropyBits);
	const entropyThreshold = finite(reading?.entropyThreshold);
	const inhibition =
		entropyBits !== null && entropyThreshold !== null && entropyThreshold > 0
			? `${Math.round(clamp((entropyBits / entropyThreshold) * 100, 0, 100))}%`
			: "—";
	const remPhaseColor =
		reading?.sideline === true
			? "var(--down)"
			: reading?.ambiguous === true
				? "var(--warn)"
				: "var(--info)";

	return (
		<div className="flex flex-col gap-3.5">
			<div className="rounded-[4px] border border-(--line) bg-(--sunken) p-3">
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">
						Attractor basin · classify
					</span>
					<span className="rounded-[2px] border border-[color-mix(in_srgb,var(--acc)_38%,transparent)] bg-[color-mix(in_srgb,var(--acc)_12%,transparent)] px-2 py-0.5 font-semibold text-[10px] text-(--acc) uppercase">
						{posterior.winner} {posterior.winnerPercent}
					</span>
				</div>
				<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
					softmax posterior · b/[class]/[sequence]
				</div>
				<div className="flex flex-col gap-2">
					{posterior.classes.length === 0 ? (
						<div className="font-mono text-[10px] text-(--f4)">
							waiting for attractor basin
						</div>
					) : (
						posterior.classes.map((row) => (
							<div key={row.name} className="flex items-center gap-2">
								<span
									className="w-16 font-mono text-[10px]"
									style={{ color: row.foreground }}
								>
									{row.name}
								</span>
								<div className="h-1.5 flex-1 overflow-hidden rounded-[3px] bg-(--line)">
									<div
										className="h-full transition-[width] duration-500"
										style={{ width: `${row.percent}%`, background: row.color }}
									/>
								</div>
								<span className="w-8 text-right font-mono text-[10px] text-(--f2)">
									{row.percent}%
								</span>
							</div>
						))
					)}
				</div>
			</div>

			<div className="rounded-[4px] border border-(--line) bg-(--sunken) p-3">
				<div className="font-semibold text-[12px] text-(--f1)">
					Contrastive evidence
				</div>
				<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
					routing margin · winner vs runner-up
				</div>
				<div className="grid grid-cols-3 gap-2.5 text-center">
					<StatBlock
						label="winner bits"
						value={posterior.winnerBits}
						tone="var(--up)"
						large
					/>
					<StatBlock
						label="runner-up bits"
						value={posterior.runnerBits}
						tone="var(--f2)"
						large
					/>
					<StatBlock
						label="KL divergence"
						value={posterior.kl}
						tone="var(--acc)"
						large
					/>
				</div>
				<div className="mt-3">
					<div className="mb-1 flex justify-between text-[9.5px] text-(--f4)">
						<span>separation margin</span>
						<span className="font-mono">{posterior.marginPercent}%</span>
					</div>
					<div className="h-[5px] overflow-hidden rounded-[3px] bg-(--line)">
						<div
							className="h-full bg-(--acc)"
							style={{ width: `${posterior.marginPercent}%` }}
						/>
					</div>
				</div>
			</div>

			<div className="rounded-[4px] border border-(--line) bg-(--sunken) p-3">
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">
						Branch entropy gate
					</span>
					<StatusBadge
						label={posterior.ambiguous ? "ambiguous" : "decisive"}
						tone={posterior.ambiguous ? "var(--down)" : "var(--up)"}
					/>
				</div>
				<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
					shannon H vs uniform threshold
				</div>
				<div className="flex items-center gap-2">
					<span className="w-[38px] font-mono text-[10px] text-(--f3)">
						{posterior.entropy}b
					</span>
					<div className="relative h-1.5 flex-1 overflow-hidden rounded-[3px] bg-(--line)">
						<div
							className="h-full"
							style={{
								width: `${posterior.entropyPercent}%`,
								background: posterior.ambiguous ? "var(--down)" : "var(--up)",
							}}
						/>
					</div>
					<span className="w-14 text-right font-mono text-[9px] text-(--f4)">
						thr {posterior.entropyThreshold}
					</span>
				</div>
			</div>

			<div className="rounded-[4px] border border-(--line) bg-(--sunken) p-3">
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">
						REM consolidation
					</span>
					<StatusBadge
						label={
							reading?.sideline
								? "sideline"
								: reading?.ambiguous
									? "rem-replay"
									: "awake"
						}
						tone={remPhaseColor}
					/>
				</div>
				<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
					episodic replay · decay · retroactive inhibition
				</div>
				<div className="grid grid-cols-3 gap-2 font-mono">
					<StatBlock label="decay γ" value={decay} />
					<StatBlock
						label="replays"
						value={replays === null ? "—" : replays.toString()}
					/>
					<StatBlock label="inhibition" value={inhibition} />
				</div>
			</div>
		</div>
	);
};

const StatBlock = ({
	label,
	value,
	tone = "var(--f1)",
	large = false,
}: {
	label: string;
	value: string;
	tone?: string;
	large?: boolean;
}) => (
	<div
		className={
			large
				? "px-1 py-1"
				: "rounded-[3px] border border-(--line) bg-(--surface) px-2 py-1.5"
		}
	>
		<div className="font-mono text-[8.5px] text-(--f4) uppercase tracking-[0.08em]">
			{label}
		</div>
		<div
			className={
				large
					? "mt-1 font-mono text-[19px] leading-none"
					: "mt-0.5 font-mono text-[11px]"
			}
			style={{ color: tone }}
		>
			{value}
		</div>
	</div>
);
