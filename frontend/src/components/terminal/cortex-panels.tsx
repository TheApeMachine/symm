import {
	cognitiveBeamsFromReading,
	cognitivePosteriorFromReading,
} from "#/components/terminal/cognitive-viz";

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
					{posterior.classes.map((row, index) => (
						<div key={`${row.name}-${index}`}>
							<div className="mb-1 flex justify-between font-mono text-[10px]">
								<span style={{ color: row.foreground }}>{row.name}</span>
								<span style={{ color: row.foreground }}>{row.percent}%</span>
							</div>
							<div className="h-1.5 overflow-hidden rounded-[3px] bg-(--line)">
								<div
									className="h-full"
									style={{ width: `${row.percent}%`, background: row.color }}
								/>
							</div>
						</div>
					))}
				</div>
				<div className="mt-3 grid grid-cols-3 gap-2">
					<StatBlock
						label="winner bits"
						value={posterior.winnerBits}
						tone="var(--up)"
					/>
					<StatBlock
						label="runner-up bits"
						value={posterior.runnerBits}
						tone="var(--f2)"
					/>
					<StatBlock
						label="KL divergence"
						value={posterior.kl}
						tone="var(--acc)"
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
						Contrastive evidence
					</span>
					<span className="font-mono text-[10px] text-(--f3)">
						{reading !== null && typeof reading.contrastEvidence === "number"
							? reading.contrastEvidence.toFixed(3)
							: "waiting"}
					</span>
				</div>
				<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
					divergence between winner and runner-up class
				</div>
				<div className="h-1.5 overflow-hidden rounded-[3px] bg-(--line)">
					<div
						className="h-full bg-info"
						style={{
							width: `${Math.round((typeof reading?.contrastEvidence === "number" ? reading.contrastEvidence : 0) * 100)}%`,
						}}
					/>
				</div>
			</div>

			<div className="rounded-[4px] border border-(--line) bg-(--sunken) p-3">
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">
						Branch entropy gate
					</span>
					<span
						className="rounded-full border px-2 py-px font-semibold text-[9px] uppercase"
						style={{
							borderColor: posterior.ambiguous ? "var(--down)" : "var(--up)",
							color: posterior.ambiguous ? "var(--down)" : "var(--up)",
						}}
					>
						{posterior.ambiguous ? "ambiguous" : "decisive"}
					</span>
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
					<span
						className="rounded-full border border-(--line2) px-2 py-px font-mono text-[9px]"
						style={{ color: remPhaseColor }}
					>
						{reading?.sideline
							? "sideline"
							: reading?.ambiguous
								? "rem-replay"
								: "awake"}
					</span>
				</div>
				<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
					decay · replays · inhibition from cognitive frame
				</div>
				<div className="grid grid-cols-3 gap-2 font-mono">
					<StatBlock
						label="decay γ"
						value={reading !== null ? reading.contrastEvidence.toFixed(3) : "—"}
					/>
					<StatBlock
						label="replays"
						value={reading !== null ? reading.lookaheadPaths.toString() : "—"}
					/>
					<StatBlock
						label="inhibition"
						value={
							reading !== null && reading.entropyThreshold > 0
								? `${Math.round((reading.entropyBits / reading.entropyThreshold) * 100)}%`
								: "—"
						}
					/>
				</div>
			</div>
		</div>
	);
};

const StatBlock = ({
	label,
	value,
	tone = "var(--f1)",
}: {
	label: string;
	value: string;
	tone?: string;
}) => (
	<div className="rounded-[3px] border border-(--line) bg-(--surface) px-2 py-1.5">
		<div className="font-mono text-[8.5px] text-(--f4) uppercase tracking-[0.08em]">
			{label}
		</div>
		<div className="mt-0.5 font-mono text-[11px]" style={{ color: tone }}>
			{value}
		</div>
	</div>
);
