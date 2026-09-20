import { useSelector } from "@tanstack/react-store";
import { focusAtom, signals } from "#/collections/app";
import {
	getRetainedResonance,
	retainResonanceRow,
} from "#/components/terminal/xray-view";
import { DataRow } from "#/components/ui/data-row";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { usePaintStore } from "#/components/ui/paint";
import { Panel } from "#/components/ui/panel";

const num = (value: unknown, digits: number): string =>
	typeof value !== "number" || !Number.isFinite(value)
		? "—"
		: value.toFixed(digits);

const ROWS = [
	{
		label: "energy",
		read: (row: Record<string, unknown>) => num(row.energy, 3),
	},
	{
		label: "surprise",
		read: (row: Record<string, unknown>) => num(row.surprise, 3),
	},
	{
		label: "base alpha",
		read: (row: Record<string, unknown>) =>
			Array.isArray(row.forwardCurve) && row.forwardCurve.length > 0
				? num(row.forwardCurve[row.forwardCurve.length - 1], 3)
				: "—",
	},
	{
		label: "horizon",
		read: (row: Record<string, unknown>) =>
			row.supportedHorizon != null
				? `${num(Number(row.supportedHorizon), 0)} ticks`
				: "—",
	},
	{
		label: "reach",
		read: (row: Record<string, unknown>) =>
			Array.isArray(row.forwardCurve)
				? `${row.forwardCurve.length} ticks`
				: "—",
	},
	{
		label: "samples",
		read: (row: Record<string, unknown>) =>
			row.resolvedSteps != null ? num(Number(row.resolvedSteps), 0) : "—",
	},
	{
		label: "task skill",
		read: (row: Record<string, unknown>) => num(row.taskSkill, 3),
	},
	{
		label: "task scale",
		read: (row: Record<string, unknown>) => num(row.taskRelativePrecision, 8),
	},
] as const;

const DYNAMICS_FIELDS = [
	{
		label: "velocity",
		read: (d: Record<string, unknown> | null | undefined) =>
			num(d?.velocity, 4),
	},
	{
		label: "acceleration",
		read: (d: Record<string, unknown> | null | undefined) =>
			num(d?.acceleration, 4),
	},
	{
		label: "liquid memory",
		read: (d: Record<string, unknown> | null | undefined) => num(d?.memory, 4),
	},
	{
		label: "memory scale",
		read: (d: Record<string, unknown> | null | undefined) =>
			num(d?.memoryScale, 4),
	},
	{
		label: "stored energy",
		read: (d: Record<string, unknown> | null | undefined) =>
			num(d?.storedEnergy, 4),
	},
	{
		label: "supplied power",
		read: (d: Record<string, unknown> | null | undefined) =>
			num(d?.suppliedPower, 4),
	},
	{
		label: "dissipation",
		read: (d: Record<string, unknown> | null | undefined) =>
			num(d?.dissipation, 4),
	},
	{
		label: "passivity residue",
		read: (d: Record<string, unknown> | null | undefined) =>
			num(d?.passivityResidue, 4),
	},
	{
		label: "diffusion variance",
		read: (d: Record<string, unknown> | null | undefined) =>
			num(d?.continuousVariance, 6),
	},
	{
		label: "jump amplitude",
		read: (d: Record<string, unknown> | null | undefined) =>
			num(d?.jumpAmplitude, 6),
	},
	{
		label: "jump variance",
		read: (d: Record<string, unknown> | null | undefined) =>
			num(d?.jumpVariance, 6),
	},
	{
		label: "rotor norm",
		read: (d: Record<string, unknown> | null | undefined) =>
			num(d?.equivarianceNorm, 4),
	},
] as const;

export const XrayManifoldPanel = () => {
	const focusSymbol = useSelector(focusAtom, (state) => state);

	const rootRef = usePaintStore(
		signals.resonance,
		(state) => {
			const ring = state[focusSymbol];
			const last = ring && !ring.isEmpty() ? ring.getLast() : null;

			if (last) {
				const row = (
					typeof (last as any).unpack === "function"
						? (last as any).unpack()
						: last
				) as unknown as Record<string, unknown>;
				const sym = typeof row.symbol === "string" ? row.symbol : "";

				if (sym) {
					retainResonanceRow(sym, row);
				}
			}

			const targetRow = getRetainedResonance(focusSymbol);
			const fields: Record<string, string> = {};

			for (const index of ROWS.keys()) {
				fields[`r${index}`] = "—";
			}

			for (const index of DYNAMICS_FIELDS.keys()) {
				fields[`d${index}`] = "—";
			}

			if (targetRow) {
				for (const [index, entry] of ROWS.entries()) {
					fields[`r${index}`] = entry.read(targetRow);
				}

				const dyn = targetRow.dynamicsNamed as
					| Record<string, unknown>
					| undefined;
				for (const [index, entry] of DYNAMICS_FIELDS.entries()) {
					fields[`d${index}`] = entry.read(dyn);
				}
			}

			return { fields };
		},
		[focusSymbol],
	);

	return (
		<Flex.Column
			ref={rootRef}
			gap={2}
			className="border-(--line) border-t px-3.5 py-3"
		>
			<Panel.Title className="text-[10px] uppercase tracking-[0.13em] text-(--f3)">
				Manifold reading
			</Panel.Title>
			<Panel.Caption className="mt-0.5 mb-1">
				settled predictive state · strict-prior direction resolution
			</Panel.Caption>

			<Grid
				cols={2}
				gap={2}
				responsive={false}
				className="gap-x-4 font-mono text-[11px]"
			>
				{ROWS.map((row, index) => (
					<DataRow
						key={row.label}
						label={row.label}
						value="—"
						paintKey={`r${index}`}
						density="bare"
						tone="f1"
					/>
				))}
			</Grid>

			<div className="mt-1 border-(--line) border-t pt-2">
				<Panel.Title className="mb-2 block text-[10px] uppercase tracking-[0.13em] text-(--f3)">
					Continuous dynamics
				</Panel.Title>
				<Grid
					cols={2}
					gap={2}
					responsive={false}
					className="gap-x-4 font-mono text-[11px]"
				>
					{DYNAMICS_FIELDS.map((field, index) => (
						<DataRow
							key={field.label}
							label={field.label}
							value="—"
							paintKey={`d${index}`}
							density="bare"
							tone="f1"
						/>
					))}
				</Grid>
			</div>
		</Flex.Column>
	);
};
