import { useSelector } from "@tanstack/react-store";
import { atoms, signals } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { asFinite } from "#/lib/format";
import {
	parseResonanceData,
	type ResonanceData,
	resonanceAt,
} from "#/lib/resonance";

const num = (value: unknown, digits: number): string => {
	const n = asFinite(value);
	return n === null ? "—" : n.toFixed(digits);
};

const ROWS = [
	{
		label: "energy",
		read: (row: ResonanceData) => num(row.energy, 3),
	},
	{
		label: "surprise",
		read: (row: ResonanceData) => num(row.surprise, 3),
	},
	{
		label: "base alpha",
		read: (row: ResonanceData) =>
			row.forwardCurve.length > 0
				? num(row.forwardCurve[row.forwardCurve.length - 1], 3)
				: "—",
	},
	{
		label: "horizon",
		read: (row: ResonanceData) =>
			row.supportedHorizon != null
				? `${num(row.supportedHorizon, 0)} ticks`
				: "—",
	},
	{
		label: "reach",
		read: (row: ResonanceData) => `${row.forwardCurve.length} ticks`,
	},
	{
		label: "task skill",
		read: (row: ResonanceData) => num(row.taskSkill, 3),
	},
	{
		label: "task scale",
		read: (row: ResonanceData) => num(row.taskRelativePrecision, 8),
	},
] as const;

export const XrayManifoldPanel = () => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const row = useSelector(signals.resonance, (state) =>
		resonanceAt(state, focusSymbol),
	);
	const res = row ? parseResonanceData(row) : undefined;

	return (
		<Flex.Column
			gap={2}
			className="border-(--line) border-t px-3.5 py-3"
		>
			<div>
				<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Manifold reading
				</div>
				<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
					settled predictive state · strict-prior direction resolution
				</div>
			</div>
			<div className="grid grid-cols-2 gap-x-4 gap-y-2 font-mono text-[11px]">
				{ROWS.map((entry) => (
					<Flex.Row key={entry.label} justify="between" gap={3}>
						<span className="text-(--f3)">{entry.label}</span>
						<Typography.Span className="text-right text-(--f1)">
							{res ? entry.read(res) : "—"}
						</Typography.Span>
					</Flex.Row>
				))}
			</div>
		</Flex.Column>
	);
};