import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { focusAtom, resonanceStore } from "#/collections/app";
import { parseResonanceData, type ResonanceData } from "#/components/charts/prediction";
import {
	getRetainedResonance,
	retainResonanceRow,
} from "#/components/terminal/xray-view";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";

const num = (value: unknown, digits: number): string =>
	typeof value !== "number" || !Number.isFinite(value)
		? "—"
		: value.toFixed(digits);

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
			Array.isArray(row.forwardCurve) && row.forwardCurve.length > 0
				? num(row.forwardCurve[row.forwardCurve.length - 1], 3)
				: "—",
	},
	{
		label: "horizon",
		read: (row: ResonanceData) =>
			row.supportedHorizon != null
				? `${num(Number(row.supportedHorizon), 0)} ticks`
				: "—",
	},
	{
		label: "reach",
		read: (row: ResonanceData) =>
			Array.isArray(row.forwardCurve)
				? `${row.forwardCurve.length} ticks`
				: "—",
	},
	{
		label: "samples",
		read: (row: ResonanceData) =>
			row.resolvedSteps != null ? num(Number(row.resolvedSteps), 0) : "—",
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

const DYNAMICS_FIELDS = [
	{
		label: "velocity",
		read: (d: Record<string, number> | null | undefined) =>
			num(d?.velocity, 4),
	},
	{
		label: "acceleration",
		read: (d: Record<string, number> | null | undefined) =>
			num(d?.acceleration, 4),
	},
	{
		label: "liquid memory",
		read: (d: Record<string, number> | null | undefined) => num(d?.memory, 4),
	},
	{
		label: "memory scale",
		read: (d: Record<string, number> | null | undefined) =>
			num(d?.memoryScale, 4),
	},
	{
		label: "stored energy",
		read: (d: Record<string, number> | null | undefined) =>
			num(d?.storedEnergy, 4),
	},
	{
		label: "supplied power",
		read: (d: Record<string, number> | null | undefined) =>
			num(d?.suppliedPower, 4),
	},
	{
		label: "dissipation",
		read: (d: Record<string, number> | null | undefined) =>
			num(d?.dissipation, 4),
	},
	{
		label: "passivity residue",
		read: (d: Record<string, number> | null | undefined) =>
			num(d?.passivityResidue, 4),
	},
	{
		label: "diffusion variance",
		read: (d: Record<string, number> | null | undefined) =>
			num(d?.continuousVariance, 6),
	},
	{
		label: "jump amplitude",
		read: (d: Record<string, number> | null | undefined) =>
			num(d?.jumpAmplitude, 6),
	},
	{
		label: "jump variance",
		read: (d: Record<string, number> | null | undefined) =>
			num(d?.jumpVariance, 6),
	},
	{
		label: "rotor norm",
		read: (d: Record<string, number> | null | undefined) =>
			num(d?.equivarianceNorm, 4),
	},
] as const;

export const XrayManifoldPanel = () => {
	const focusSymbol = useSelector(focusAtom);
	const root = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const updateFromState = () => {
			if (!root.current) return;
			const ring = resonanceStore.state[focusSymbol];
			const last = ring && !ring.isEmpty() ? ring.getLast() : null;

			if (last) {
				const sym = typeof last.symbol === "string" ? last.symbol : "";

				if (sym) {
					retainResonanceRow(sym, last as unknown as Record<string, unknown>);
				}
			}

			const retained = getRetainedResonance(focusSymbol);
			const targetRow = parseResonanceData((retained as unknown as MeasurementT | null) ?? last ?? undefined);

			const set = (q: string, value: string) => {
				const el = root.current?.querySelector<HTMLElement>(`[data-f="${q}"]`);
				if (el) el.textContent = value;
			};

			for (const index of ROWS.keys()) {
				set(`r${index}`, "—");
			}

			for (const index of DYNAMICS_FIELDS.keys()) {
				set(`d${index}`, "—");
			}

			if (targetRow) {
				for (const [index, entry] of ROWS.entries()) {
					set(`r${index}`, entry.read(targetRow));
				}

				const dyn = targetRow.dynamics;
				for (const [index, entry] of DYNAMICS_FIELDS.entries()) {
					set(`d${index}`, entry.read(dyn));
				}
			}
		};

		updateFromState();
		const subscription = resonanceStore.subscribe(updateFromState);

		return () => {
			subscription.unsubscribe();
		};
	}, [focusSymbol]);

	return (
		<Flex.Column
			ref={root}
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
				{ROWS.map((row, index) => (
					<Flex.Row key={row.label} justify="between" gap={3}>
						<span className="text-(--f3)">{row.label}</span>
						<Typography.Span
							data-f={`r${index}`}
							className="text-right text-(--f1)"
						>
							—
						</Typography.Span>
					</Flex.Row>
				))}
			</div>
			<div className="mt-1 border-(--line) border-t pt-2">
				<div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Continuous dynamics
				</div>
				<div className="grid grid-cols-2 gap-x-4 gap-y-2 font-mono text-[11px]">
					{DYNAMICS_FIELDS.map((field, index) => (
						<div key={field.label} className="flex justify-between gap-3">
							<span className="text-(--f3)">{field.label}</span>
							<Typography.Span
								data-f={`d${index}`}
								className="text-right text-(--f1)"
							>
								—
							</Typography.Span>
						</div>
					))}
				</div>
			</div>
		</Flex.Column>
	);
};
