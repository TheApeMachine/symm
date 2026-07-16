import {
	cognitiveBeamsFromReading,
	cognitivePosteriorFromReading,
} from "#/components/terminal/cognitive-viz";
import { Badge } from "@/components/ui/badge";
import { Meter } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import { Stat } from "@/components/ui/stat";

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
				<Panel key={beam.rank} size="s" className="flex items-center gap-2">
					<span
						className={
							beam.variant === "warning"
								? "w-4 shrink-0 font-mono text-[10px] text-(--acc)"
								: "w-4 shrink-0 font-mono text-[10px] text-(--info)"
						}
					>
						#{beam.rank}
					</span>
					<span className="flex-1 font-mono text-[11px] text-(--f1)">
						{beam.sequence || "root"}
					</span>
					<Meter
						layout="bar"
						percent={beam.percent}
						variant={beam.variant}
						trackClassName="w-[70px]"
						size="xs"
					/>
					<span className="w-11 shrink-0 text-right font-mono text-[9.5px] text-(--f3)">
						{beam.score}
					</span>
				</Panel>
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
	const replays = finite(reading?.lookaheadPaths);
	const nodeCount = finite(reading?.nodeCount);
	const validReplays = replays !== null && replays >= 0 ? replays : null;
	const validNodeCount =
		nodeCount !== null && nodeCount >= 0 ? nodeCount : null;
	const decay =
		validReplays !== null &&
		validNodeCount !== null &&
		validReplays + validNodeCount > 0
			? (validReplays / (validReplays + validNodeCount)).toFixed(3)
			: lookaheadScore === null
				? "—"
				: Math.exp(Math.min(0, lookaheadScore)).toFixed(3);
	const entropyBits = finite(reading?.entropyBits);
	const entropyThreshold = finite(reading?.entropyThreshold);
	const inhibition =
		entropyBits !== null && entropyThreshold !== null && entropyThreshold > 0
			? `${Math.round(clamp((entropyBits / entropyThreshold) * 100, 0, 100))}%`
			: "—";
	const remPhaseVariant = reading?.sideline
		? "error"
		: reading?.ambiguous
			? "warning"
			: "info";

	return (
		<div className="flex flex-col gap-3.5">
			<Panel>
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">
						Attractor basin · classify
					</span>
					<Badge
						label={
							posterior.classes.length === 0
								? `waiting ${posterior.winnerPercent}`
								: `${posterior.winner} ${posterior.winnerPercent}`
						}
						variant="warning"
					/>
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
									className={
										row.emphasis
											? "w-16 font-mono text-[10px] text-(--f1)"
											: "w-16 font-mono text-[10px] text-(--f3)"
									}
								>
									{row.name}
								</span>
								<Meter
									layout="bar"
									percent={row.percent}
									variant={row.variant}
									trackClassName="flex-1"
									size="m"
									animated
								/>
								<span className="w-8 text-right font-mono text-[10px] text-(--f2)">
									{row.percent}%
								</span>
							</div>
						))
					)}
				</div>
			</Panel>

			<Panel>
				<div className="font-semibold text-[12px] text-(--f1)">
					Contrastive evidence
				</div>
				<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
					routing margin · winner vs runner-up
				</div>
				<div className="grid grid-cols-3 gap-2.5 text-center">
					<Stat
						layout="feature"
						label="winner bits"
						value={posterior.winnerBits}
						variant="success"
					/>
					<Stat
						layout="feature"
						label="runner-up bits"
						value={posterior.runnerBits}
					/>
					<Stat
						layout="feature"
						label="KL divergence"
						value={posterior.kl}
						variant="warning"
					/>
				</div>
				<Meter
					layout="stacked"
					label="separation margin"
					value={`${posterior.marginPercent}%`}
					percent={posterior.marginPercent}
					variant="warning"
					size="s"
					className="mt-3"
					labelClassName="text-[9.5px] text-(--f4)"
				/>
			</Panel>

			<Panel>
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">
						Branch entropy gate
					</span>
					<Badge
						label={posterior.ambiguous ? "ambiguous" : "decisive"}
						variant={posterior.ambiguous ? "error" : "success"}
					/>
				</div>
				<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
					shannon H vs uniform threshold
				</div>
				<Meter
					layout="inline"
					label={`${posterior.entropy}b`}
					value={`thr ${posterior.entropyThreshold}`}
					percent={posterior.entropyPercent}
					variant={posterior.ambiguous ? "error" : "success"}
					size="m"
				/>
			</Panel>

			<Panel>
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">
						REM consolidation
					</span>
					<Badge
						label={
							reading?.sideline
								? "waiting"
								: reading?.ambiguous
									? "rem-replay"
									: "awake"
						}
						variant={remPhaseVariant}
					/>
				</div>
				<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
					episodic replay · decay · retroactive inhibition
				</div>
				<div className="grid grid-cols-3 gap-2 font-mono">
					<Stat layout="tile" label="decay γ" value={decay} />
					<Stat
						layout="tile"
						label="replays"
						value={replays === null ? "—" : replays.toString()}
					/>
					<Stat layout="tile" label="inhibition" value={inhibition} />
				</div>
			</Panel>
		</div>
	);
};
