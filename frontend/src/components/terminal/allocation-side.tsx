import { useSelector } from "@tanstack/react-store";
import { balancesStore } from "#/collections/balances";
import { measurementsStore } from "#/collections/measurements";
import { playbookStore } from "#/collections/playbook";
import {
	allocationEntryStats,
	fixed,
} from "#/components/terminal/decision-format";
import { terminalDecisionsFromWalk } from "#/components/terminal/decisions-from-walk";
import { kernelsForFocus } from "#/components/terminal/kernels";
import type { TerminalModel } from "#/components/terminal/model";

export type AllocationCandidate = {
	key: string;
	symbol: string;
	scoreValue: number;
	edge: number;
	share: number;
	notional: number;
	allocated: boolean;
};

export type AllocationResult = {
	threshold: number;
	median: number;
	mad: number;
	candidates: AllocationCandidate[];
	deployed: number;
	deployedPercent: number;
	freeCash: number;
};

const parseCurrency = (value: string): number => {
	const numeric = Number(value.replace(/[^0-9.-]/g, ""));

	return Number.isFinite(numeric) ? numeric : 0;
};

/*
allocationRows applies edge-proportional sizing to the candidate decisions: the
median+MAD entry gate selects deployable candidates, each one's edge over the
gate becomes its share of the positive-edge mass, and notional is the deployable
free cash times that share. Every figure is derived from the live distribution —
no fixed slot sizes.
*/
export const allocationRows = (model: TerminalModel): AllocationResult => {
	const decisions = model.decisions ?? [];
	const scores = decisions.map((decision) => decision.scoreValue);
	const stats = allocationEntryStats(scores);
	const freeCash = parseCurrency(model.wallet?.available ?? "0");

	const edges = decisions.map((decision) => ({
		decision,
		edge: decision.scoreValue - stats.threshold,
	}));

	// A candidate clears the gate when its score reaches the entry line (edge ≥ 0).
	// Because the line is clamped to the top score, at least the strongest
	// candidate always qualifies. Shares are edge-proportional; when every
	// qualifying edge is zero (a single standout at the clamped line) the
	// qualifiers split free cash evenly so the best opportunity is still sized.
	const qualifiers = edges.filter((entry) => entry.edge >= 0);
	const positiveEdgeMass = qualifiers.reduce(
		(sum, entry) => sum + entry.edge,
		0,
	);

	const candidates: AllocationCandidate[] = edges.map(({ decision, edge }) => {
		const allocated = edge >= 0;
		const share = allocated
			? positiveEdgeMass > 0
				? edge / positiveEdgeMass
				: 1 / qualifiers.length
			: 0;
		const notional = share * freeCash;

		return {
			key: decision.key,
			symbol: decision.symbol,
			scoreValue: decision.scoreValue,
			edge,
			share,
			notional,
			allocated,
		};
	});

	const deployed = candidates.reduce(
		(sum, candidate) => sum + candidate.notional,
		0,
	);
	const deployedPercent = freeCash > 0 ? (deployed / freeCash) * 100 : 0;

	return {
		threshold: stats.threshold,
		median: stats.median,
		mad: stats.mad,
		candidates,
		deployed,
		deployedPercent,
		freeCash,
	};
};

const Bar = ({ percent, color }: { percent: number; color: string }) => (
	<div className="h-1 overflow-hidden rounded-sm bg-(--line)">
		<div
			className="h-full"
			style={{
				width: `${Math.max(0, Math.min(100, percent))}%`,
				background: color,
			}}
		/>
	</div>
);

const Stat = ({
	label,
	value,
	accent = false,
}: {
	label: string;
	value: string;
	accent?: boolean;
}) => (
	<div className="flex items-center justify-between">
		<span className="text-(--f4)">{label}</span>
		<span style={{ color: accent ? "var(--acc)" : "var(--f1)" }}>{value}</span>
	</div>
);

/*
AllocationSidePanel renders the live edge-proportional allocation: the derived
median/MAD gate, deployed fraction, and each candidate's share of free cash. It
binds to the playbook walk traces and signal kernels broadcast by the backend.
*/
export const AllocationSidePanel = () => {
	const evaluations = useSelector(playbookStore, (state) => state.evaluations);
	const readings = useSelector(measurementsStore, (state) => state);
	const balances = useSelector(balancesStore, (state) => state.frame);

	const assets =
		(balances?.asset as Array<Record<string, unknown>> | undefined) ?? [];
	const quote =
		(assets.find((asset) => asset.asset === "USD" || asset.asset === "EUR")
			?.asset as string) || "USD";
	const available = Number(
		assets.find((asset) => asset.asset === quote)?.balance ?? 0,
	);

	const kernels = kernelsForFocus(readings);
	const decisions = terminalDecisionsFromWalk(evaluations, kernels);

	if (decisions.length === 0) {
		return (
			<div className="font-mono text-(--f4) text-xs">
				waiting for allocation frames
			</div>
		);
	}

	const alloc = allocationRows({
		wallet: { available: `${available}`, reserved: "0" },
		decisions,
	});

	return (
		<div className="flex flex-col gap-3.5">
			<div className="grid grid-cols-2 gap-2 font-mono text-[10px]">
				<Stat label="entry gate" value={fixed(alloc.threshold)} accent />
				<Stat label="median" value={fixed(alloc.median)} />
				<Stat label="mad" value={fixed(alloc.mad)} />
				<Stat
					label="deployed"
					value={`${alloc.deployedPercent.toFixed(1)}%`}
				/>
			</div>

			<div className="flex flex-col gap-2">
				{alloc.candidates
					.slice()
					.sort((left, right) => right.scoreValue - left.scoreValue)
					.map((candidate) => (
						<div
							key={candidate.key}
							className="rounded-sm border border-(--line) bg-(--surface) px-2.5 py-2"
							style={{ opacity: candidate.allocated ? 1 : 0.5 }}
						>
							<div className="flex items-center justify-between font-mono text-[11px]">
								<span className="text-(--f1)">{candidate.symbol}</span>
								<span
									style={{
										color: candidate.allocated ? "var(--up)" : "var(--f4)",
									}}
								>
									{candidate.allocated
										? `${(candidate.share * 100).toFixed(1)}%`
										: "—"}
								</span>
							</div>
							<div className="mt-1.5">
								<Bar
									percent={candidate.share * 100}
									color={candidate.allocated ? "var(--up)" : "var(--line2)"}
								/>
							</div>
							<div className="mt-1.5 flex items-center justify-between font-mono text-[9px] text-(--f4)">
								<span>edge {fixed(candidate.edge)}</span>
								<span>
									{candidate.notional.toFixed(2)} {quote}
								</span>
							</div>
						</div>
					))}
			</div>
		</div>
	);
};
