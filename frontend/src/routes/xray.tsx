import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import { resonanceStore } from "#/collections/resonance";
import { terminalStore } from "#/collections/terminal";
import { ContextStrip } from "#/components/terminal/context";
import { XrayLayerRows } from "#/components/terminal/xray-layers";

const RowFact = ({
	label,
	value,
	accent,
}: {
	label: string;
	value: unknown;
	accent?: string;
}) => (
	<div className="flex justify-between gap-3">
		<span className="text-(--f3)">{label}</span>
		<span style={{ color: accent ?? "var(--f1)" }}>
			{value === undefined || value === null ? "—" : String(value)}
		</span>
	</div>
);

const RouteComponent = () => {
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const { selectFocusSymbol } = terminalStore.actions;

	// Raw measurement frames, keyed by scope, straight from the backend.
	const hawkes = useSelector(
		measurementsStore,
		(state) => state.hawkes?.[focusSymbol],
	) as Record<string, unknown> | undefined;
	const resonanceMeas = useSelector(
		measurementsStore,
		(state) => state.resonance?.[focusSymbol],
	) as Record<string, unknown> | undefined;

	// Resonance universe snapshot (role "resonance") carries the per-layer wire.
	const resonance = useSelector(resonanceStore, (state) => state.frame);
	const focus = resonance?.focus as Record<string, unknown> | undefined;
	const layers = (focus?.layers as Record<string, unknown>[]) ?? [];

	// Manifold field snapshot (role "manifold").
	const manifold = useSelector(appStore, (state) => state.lastManifoldFrame);
	const reading = manifold?.reading as Record<string, unknown> | undefined;

	const hawkesOut = hawkes?.output as Record<string, unknown> | undefined;
	const symbols =
		(resonance?.symbols as Record<string, unknown>[] | undefined)
			?.map((entry) => entry.symbol as string)
			.filter(Boolean) ?? [];

	return (
		<div className="flex h-full min-w-[1100px] flex-col">
			<ContextStrip
				label="Inspect symbol"
				symbols={symbols.slice(0, 10)}
				activeSymbol={focusSymbol}
				onSelect={selectFocusSymbol}
			/>
			<div className="grid min-h-0 flex-1 grid-cols-[minmax(520px,1fr)_352px]">
				<div className="flex min-h-0 flex-col overflow-auto border-(--line) border-r">
					<div className="px-[18px] py-4">
						<div className="flex items-baseline justify-between gap-3">
							<span className="font-serif font-semibold text-[22px] text-(--f1) leading-[1.1]">
								Predictive-coding hierarchy
							</span>
							<span className="font-mono text-[11px] text-(--f3)">
								{focusSymbol}
							</span>
						</div>
						<div className="mt-1 font-mono text-[10px] text-(--f4)">
							latent state · prediction error ε per layer · macro = abstract
							regime, sensory = raw tape
						</div>
						<div className="mt-4">
							{layers.length > 0 ? (
								<XrayLayerRows layers={layers} />
							) : (
								<div className="font-mono text-[10px] text-(--f4)">
									waiting for resonance layers
								</div>
							)}
						</div>
					</div>
					<div className="border-(--line) border-t px-[18px] py-4 font-mono text-[11px]">
						<div className="mb-2 font-semibold text-[10px] text-(--f2) uppercase tracking-[0.13em]">
							Hawkes self-exciting intensity
						</div>
						<div className="flex flex-col gap-2">
							<RowFact label="λ intensity" value={hawkesOut?.intensity} />
							<RowFact label="branching η" value={hawkesOut?.branching} />
							<RowFact label="spectral radius" value={hawkesOut?.radius} />
							<RowFact label="asymmetry" value={hawkesOut?.asymmetry} />
							<RowFact label="buy intensity" value={hawkesOut?.buyIntensity} />
							<RowFact
								label="sell intensity"
								value={hawkesOut?.sellIntensity}
							/>
							<RowFact label="exogenous" value={hawkesOut?.exo} />
						</div>
					</div>
				</div>
				<div className="flex min-h-0 flex-col overflow-auto bg-(--surface)">
					<div className="px-3.5 pt-3 pb-1.5">
						<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
							Latent manifold
						</div>
						<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
							universe embedding · clustered by regime · focus pulses
						</div>
					</div>
					<div className="mt-2 flex flex-col gap-2.5 border-(--line) border-t px-3.5 py-3 font-mono text-[12px]">
						<RowFact
							label="focus symbol"
							value={resonance?.focus_symbol}
							accent="var(--acc)"
						/>
						<RowFact label="category" value={focus?.category} />
						<RowFact label="confidence" value={focus?.confidence} />
						<RowFact label="surprise" value={resonanceMeas?.surprise} />
						<RowFact label="energy" value={focus?.energy} />
						<RowFact label="symbol count" value={resonance?.symbol_count} />
					</div>
					<div className="flex flex-col gap-2 border-(--line) border-t px-3.5 py-3">
						<div>
							<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
								Manifold reading
							</div>
							<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
								navier–stokes · ρ projection · oscillator carriers
							</div>
						</div>
						<div className="grid grid-cols-2 gap-x-4 gap-y-2 font-mono text-[11px]">
							<RowFact label="∇·u" value={reading?.divergence} />
							<RowFact label="|ψ|²" value={reading?.coherence_mag2} />
							<RowFact label="guide v" value={reading?.guidance_speed} />
							<RowFact label="viscosity" value={reading?.viscosity_proxy} />
							<RowFact label="∇p norm" value={reading?.pressure_grad_norm} />
						</div>
					</div>
				</div>
			</div>
		</div>
	);
};

export const Route = createFileRoute("/xray")({
	component: RouteComponent,
});
