import { useSelector } from "@tanstack/react-store";
import { useMemo } from "react";
import { atoms, signals } from "#/collections/app";
import { heatColor } from "#/components/terminal/canvas";
import {
	keyedLayerCells,
	layerCellsFromState,
} from "#/components/terminal/xray-layers";
import {
	carrierScope,
	resolveXrayCarrier,
	xrayLayersFromResonance,
} from "#/components/terminal/xray-view";
import { resonanceUniverseRows } from "#/lib/resonance";
import { symbolScopeAliases } from "#/lib/symbol-scope";
import { Typography } from "@/components/ui/typography";

const layerErrorTone = (error: number): string => {
	if (error > 0.55) return "var(--down)";
	if (error > 0.3) return "var(--warn)";
	return "var(--up)";
};

const HierarchyRow = ({
	layer,
}: {
	layer: ReturnType<typeof xrayLayersFromResonance>[number];
}) => {
	const cells = keyedLayerCells(layer.label, layerCellsFromState(layer.state));
	const errorPct =
		layer.error_norm !== null
			? Math.min(100, Math.max(0, layer.error_norm * 100))
			: 0;
	const errorColor =
		layer.error_norm !== null
			? layerErrorTone(layer.error_norm)
			: "transparent";

	return (
		<div className="flex items-center gap-3">
			<span className="w-23 shrink-0 font-mono text-[10px] text-(--f3)">
				{layer.label}
			</span>
			<div className="grid flex-1 grid-cols-16 gap-0.5">
				{cells.map((cell) => (
					<div
						key={cell.key}
						className="min-w-0 aspect-square rounded-[1px] transition-colors duration-150 ease-out"
						style={{ background: heatColor((cell.value + 1) / 2) }}
					/>
				))}
			</div>
			<div className="w-20 shrink-0">
				<div className="flex justify-between font-mono text-[9px] text-(--f4)">
					<span>ε</span>
					<span
						style={{
							color: layer.error_norm !== null ? errorColor : undefined,
						}}
					>
						{layer.error_norm !== null ? layer.error_norm.toFixed(3) : "—"}
					</span>
				</div>
				<div className="mt-0.75 h-1 overflow-hidden rounded-xs bg-(--line)">
					<div
						className="h-full"
						style={{
							width: `${errorPct}%`,
							background: errorColor,
						}}
					/>
				</div>
			</div>
		</div>
	);
};

export const XrayHierarchyPanel = () => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const universe = useSelector(signals.resonance, resonanceUniverseRows);

	const { layers, waiting } = useMemo(() => {
		if (universe.length === 0) {
			return {
				layers: [] as ReturnType<typeof xrayLayersFromResonance>,
				waiting: "waiting for resonance layers",
			};
		}

		const carrier = resolveXrayCarrier(universe, focusSymbol);
		const parsed = xrayLayersFromResonance(carrier);
		const carrierSymbol = carrier ? carrierScope(carrier) : "";

		if (parsed.length === 0) {
			return {
				layers: parsed,
				waiting: `resonance carrier for ${focusSymbol} has no layer rows yet`,
			};
		}

		if (
			carrierSymbol !== "" &&
			carrierSymbol !== focusSymbol &&
			!symbolScopeAliases(focusSymbol).includes(carrierSymbol)
		) {
			return {
				layers: parsed,
				waiting: `layers from ${carrierSymbol} · focus ${focusSymbol}`,
			};
		}

		return { layers: parsed, waiting: "" as string };
	}, [universe, focusSymbol]);

	return (
		<div className="shrink-0 px-4.5 py-4">
			<div className="flex items-baseline justify-between gap-3">
				<Typography.Display size="lg">
					Predictive-coding hierarchy
				</Typography.Display>
				<span className="shrink-0 font-mono text-[11px] text-(--f3)">
					{focusSymbol}
				</span>
			</div>
			<div className="mt-1 font-mono text-[10px] text-(--f4)">
				latent state · prediction error ε per layer · macro = abstract regime,
				sensory = raw tape
			</div>
			<div className="mt-4">
				{waiting !== "" ? (
					<div className="font-mono text-[10px] text-(--f4)">{waiting}</div>
				) : null}
				{layers.length > 0 ? (
					<div className="flex flex-col gap-2">
						{layers.map((layer) => (
							<HierarchyRow key={layer.label} layer={layer} />
						))}
					</div>
				) : null}
			</div>
		</div>
	);
};
