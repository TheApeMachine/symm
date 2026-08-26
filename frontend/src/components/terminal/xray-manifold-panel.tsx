import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
import { focusStore, resonanceStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { Resonance } from "#/providers/telemetry/telemetry/resonance";
import { ResonanceDynamics } from "#/providers/telemetry/telemetry/resonance-dynamics";
import { ResonanceForecast } from "#/providers/telemetry/telemetry/resonance-forecast";

const num = (value: number | undefined, digits: number): string =>
	value === undefined || !Number.isFinite(value) ? "—" : value.toFixed(digits);

const ROWS = [
	{ label: "energy", read: (row: Resonance) => num(row.energy(), 3) },
	{ label: "surprise", read: (row: Resonance) => num(row.surprise(), 3) },
	{ label: "base alpha", read: (_row: Resonance) => "—" },
	{ label: "horizon", read: (_row: Resonance, fcast: ResonanceForecast | null) => `${num(fcast ? Number(fcast.supportedHorizon()) : undefined, 0)} ticks` },
	{ label: "reach", read: (_row: Resonance, fcast: ResonanceForecast | null) => `${num(fcast ? Number(fcast.probeHorizon()) : undefined, 0)} ticks` },
	{ label: "samples", read: (row: Resonance) => num(Number(row.samples()), 0) },

	{ label: "task skill", read: (row: Resonance) => num(row.taskSkill(), 3) },
	{ label: "task scale", read: (row: Resonance) => num(row.taskRelativePrecision(), 8) },
] as const;

const DYNAMICS_FIELDS = [
	{ label: "velocity", read: (d: ResonanceDynamics | null) => num(d?.velocity(), 4) },
	{ label: "acceleration", read: (d: ResonanceDynamics | null) => num(d?.acceleration(), 4) },
	{ label: "liquid memory", read: (d: ResonanceDynamics | null) => num(d?.memory(), 4) },
	{ label: "memory scale", read: (d: ResonanceDynamics | null) => num(d?.memoryScale(), 4) },
	{ label: "stored energy", read: (d: ResonanceDynamics | null) => num(d?.storedEnergy(), 4) },
	{ label: "supplied power", read: (d: ResonanceDynamics | null) => num(d?.suppliedPower(), 4) },
	{ label: "dissipation", read: (d: ResonanceDynamics | null) => num(d?.dissipation(), 4) },
	{ label: "passivity residue", read: (d: ResonanceDynamics | null) => num(d?.passivityResidue(), 4) },
	{ label: "diffusion variance", read: (d: ResonanceDynamics | null) => num(d?.continuousVariance(), 6) },
	{ label: "jump amplitude", read: (d: ResonanceDynamics | null) => num(d?.jumpAmplitude(), 6) },
	{ label: "jump variance", read: (d: ResonanceDynamics | null) => num(d?.jumpVariance(), 6) },
	{ label: "rotor norm", read: (d: ResonanceDynamics | null) => num(d?.equivarianceNorm(), 4) },
] as const;

const resObj = new Resonance();
const fcastObj = new ResonanceForecast();
const dynObj = new ResonanceDynamics();

export const XrayManifoldPanel = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const root = useRef<HTMLDivElement>(null);

	resonanceStore.subscribe((state) => {
		if (!root.current) return;
		const last = state.getLast();
		if (!last) return;

		let targetRow: Resonance | null = null;
		for (let i = 0; i < last.rowsLength(); i++) {
			const row = last.rows(i, resObj);
			if (row && row.symbol() === focusSymbol) {
				targetRow = row;
				break;
			}
		}

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f="${q}"]`);
			if (el) el.textContent = value;
		};

		if (!targetRow) {
			for (const [index] of ROWS.entries()) {
				set(`r${index}`, "—");
			}
			for (const [index] of DYNAMICS_FIELDS.entries()) {
				set(`d${index}`, "—");
			}
			return;
		}

		const fcast = targetRow.forecast(fcastObj);
		const dyn = targetRow.dynamics(dynObj);

		for (const [index, entry] of ROWS.entries()) {
			set(`r${index}`, entry.read(targetRow, fcast));
		}

		for (const [index, entry] of DYNAMICS_FIELDS.entries()) {
			set(`d${index}`, entry.read(dyn));
		}
	});

	return (
		<Flex.Column ref={root} gap={2} className="flex flex-col gap-2 border-(--line) border-t px-3.5 py-3">
			<div>
				<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">Manifold reading</div>
				<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">settled predictive state · strict-prior direction resolution</div>
			</div>
			<div className="grid grid-cols-2 gap-x-4 gap-y-2 font-mono text-[11px]">
				{ROWS.map((row, index) => (
					<Flex.Row key={row.label} justify="between" gap={3}>
						<span className="text-(--f3)">{row.label}</span>
						<Typography.Span data-f={`r${index}`} className="text-right text-(--f1)">—</Typography.Span>
					</Flex.Row>
				))}
			</div>
			<div className="mt-1 border-(--line) border-t pt-2">
				<div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">Continuous dynamics</div>
				<div className="grid grid-cols-2 gap-x-4 gap-y-2 font-mono text-[11px]">
					{DYNAMICS_FIELDS.map((field, index) => (
						<div key={field.label} className="flex justify-between gap-3">
							<span className="text-(--f3)">{field.label}</span>
							<Typography.Span data-f={`d${index}`} className="text-right text-(--f1)">—</Typography.Span>
						</div>
					))}
				</div>
			</div>
		</Flex.Column>
	);
};