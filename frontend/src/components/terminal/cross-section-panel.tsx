import { useSelector } from "@tanstack/react-store";
import { diagnosticsStore } from "#/collections/diagnostics";
import { StatusBadge } from "#/components/dashboard/status-badge";
import { Meter, Stat } from "#/components/terminal/health";

const finiteNumber = (value: unknown): number =>
	typeof value === "number" && Number.isFinite(value) ? value : 0;

const stringArray = (value: unknown): string[] =>
	Array.isArray(value)
		? value.filter((entry): entry is string => typeof entry === "string")
		: [];

const numberArray = (value: unknown): number[] =>
	Array.isArray(value)
		? value.filter(
				(entry): entry is number =>
					typeof entry === "number" && Number.isFinite(entry),
			)
		: [];

const median = (values: number[]): number => {
	if (values.length === 0) {
		return 0;
	}

	const sorted = [...values].sort((left, right) => left - right);
	const middle = Math.floor(sorted.length / 2);

	return sorted.length % 2 === 0
		? (sorted[middle - 1] + sorted[middle]) / 2
		: sorted[middle];
};

export const compactNumber = (value: number): string => {
	const abs = Math.abs(value);

	if (abs >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`;
	if (abs >= 1_000) return `${(value / 1_000).toFixed(1)}K`;

	return abs >= 1 ? value.toFixed(2) : value.toFixed(4);
};

export type CrossSectionReadout = {
	leader: string;
	leadershipThresholdPercent: number;
	breadthPercent: number;
	symbolCount: number;
	medianVolume: number;
	medianQuoteNotional: number;
	medianExecutableDepth: number;
};

export const crossSectionReadoutFromFrame = (
	frame: Record<string, unknown> | null,
): CrossSectionReadout => ({
	leader: typeof frame?.leader === "string" ? frame.leader : "",
	leadershipThresholdPercent: finiteNumber(frame?.leadershipThreshold) * 100,
	breadthPercent: finiteNumber(frame?.breadth) * 100,
	symbolCount: stringArray(frame?.symbols).length,
	medianVolume: median(numberArray(frame?.volumes)),
	medianQuoteNotional: median(numberArray(frame?.quoteNotionals)),
	medianExecutableDepth: median(numberArray(frame?.executableDepths)),
});

export const CrossSectionPanel = () => {
	const frame = useSelector(diagnosticsStore, (state) => state.frame);
	const readout = crossSectionReadoutFromFrame(frame);
	const broad = readout.breadthPercent >= 50;

	return (
		<div className="rounded-[4px] border border-(--line) bg-(--sunken) p-[13px]">
			<div className="flex items-center justify-between">
				<span className="font-semibold text-(--f1) text-xs">Cross-section</span>
				<StatusBadge
					label={broad ? "broad" : "thin"}
					tone={broad ? "var(--up)" : "var(--warn)"}
				/>
			</div>
			<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
				breadth · leadership · liquidity axes · {readout.symbolCount} symbols
			</div>
			<div className="flex items-center justify-between">
				<span className="font-mono text-[11px] text-(--f2)">
					leader <span className="text-(--acc)">{readout.leader || "—"}</span>
				</span>
				<span className="font-mono text-[10px] text-(--f4)">
					thr {readout.leadershipThresholdPercent.toFixed(2)}%
				</span>
			</div>
			<div className="mt-2.5">
				<Meter
					label="Breadth"
					value={`${Math.round(readout.breadthPercent)}%`}
					percent={readout.breadthPercent}
					color={broad ? "var(--up)" : "var(--warn)"}
				/>
			</div>
			<div className="mt-[13px] flex justify-between">
				<Stat value={compactNumber(readout.medianVolume)} label="med volume" />
				<Stat
					value={compactNumber(readout.medianQuoteNotional)}
					label="med notional"
				/>
				<Stat
					value={compactNumber(readout.medianExecutableDepth)}
					label="med depth"
				/>
			</div>
		</div>
	);
};
