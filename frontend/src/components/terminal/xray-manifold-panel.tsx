import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import type { ResonanceFrame } from "#/collections/types";
import { Typography } from "#/components/ui/typography";
import { Flex } from "../ui";
import { resonanceStore, useSubscribe } from "#/providers/ws-stores";

const num = (value: number | undefined, digits: number): string =>
	value === undefined ? "—" : value.toFixed(digits);

const alphaOf = (row: ResonanceFrame): number | undefined => {
	const alpha = row.alpha;

	return typeof alpha === "number" ? alpha : undefined;
};

const ROWS = [
	{ label: "energy", read: (row: ResonanceFrame) => num(row.energy, 3) },
	{ label: "surprise", read: (row: ResonanceFrame) => num(row.surprise, 3) },
	{ label: "base alpha", read: (row: ResonanceFrame) => num(alphaOf(row), 4) },
	{ label: "horizon", read: (row: ResonanceFrame) => `${num(row.forecast?.supportedHorizon, 0)} ticks` },
	{ label: "reach", read: (row: ResonanceFrame) => `${num(row.forecast?.probeHorizon, 0)} ticks` },
	{ label: "samples", read: (row: ResonanceFrame) => num(row.samples, 0) },
	{ label: "task skill", read: (row: ResonanceFrame) => num(row.taskSkill, 3) },
	{ label: "task scale", read: (row: ResonanceFrame) => num(row.taskRelativePrecision, 8) },
] as const;

const DYNAMICS_FIELDS = [
	{ label: "velocity", read: (d: ResonanceFrame["dynamics"]) => num(d?.velocity, 4) },
	{ label: "acceleration", read: (d: ResonanceFrame["dynamics"]) => num(d?.acceleration, 4) },
	{ label: "liquid memory", read: (d: ResonanceFrame["dynamics"]) => num(d?.memory, 4) },
	{ label: "memory scale", read: (d: ResonanceFrame["dynamics"]) => num(d?.memoryScale, 4) },
	{ label: "stored energy", read: (d: ResonanceFrame["dynamics"]) => num(d?.storedEnergy, 4) },
	{ label: "supplied power", read: (d: ResonanceFrame["dynamics"]) => num(d?.suppliedPower, 4) },
	{ label: "dissipation", read: (d: ResonanceFrame["dynamics"]) => num(d?.dissipation, 4) },
	{ label: "passivity residue", read: (d: ResonanceFrame["dynamics"]) => num(d?.passivityResidue, 4) },
	{ label: "diffusion variance", read: (d: ResonanceFrame["dynamics"]) => num(d?.continuousVariance, 6) },
	{ label: "jump amplitude", read: (d: ResonanceFrame["dynamics"]) => num(d?.jumpAmplitude, 6) },
	{ label: "jump variance", read: (d: ResonanceFrame["dynamics"]) => num(d?.jumpVariance, 6) },
	{ label: "rotor norm", read: (d: ResonanceFrame["dynamics"]) => num(d?.equivarianceNorm, 4) },
] as const;

export const XrayManifoldPanel = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	const root = useSubscribe(resonanceStore, (state) => {
		const row = state.resonance[focusSymbol]?.latest();

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f="${q}"]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		if (row === undefined) {
			for (const [index] of ROWS.entries()) {
				set(`r${index}`, "—");
			}

			for (const [index] of DYNAMICS_FIELDS.entries()) {
				set(`d${index}`, "—");
			}

			return;
		}

		for (const [index, entry] of ROWS.entries()) {
			set(`r${index}`, entry.read(row));
		}

		for (const [index, entry] of DYNAMICS_FIELDS.entries()) {
			set(`d${index}`, entry.read(row.dynamics));
		}
	}, [focusSymbol]);

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