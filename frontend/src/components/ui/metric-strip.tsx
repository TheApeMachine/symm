import { useSelector } from "@tanstack/react-store";
import { useMemo } from "react";
import { atoms, DEFAULT_KERNELS, kernelMeasurement, signals } from "#/collections/app";
import type { Value } from "#/types/value";
import { MetricRow } from "./metric";

/*
One workspace subscription per dashboard column: each kernel row still binds
through Component, but React reconciles one parent per tick instead of ten.
*/
export const KernelMetricStrip = () => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const tick = useSelector(
		signals.workspace,
		(state) => state.aggregate?.tick ?? 0n,
	);

	const measurements = useMemo(() => {
		const aggregate = signals.workspace.state.aggregate;
		const out: Record<string, Value | null> = {};
		for (const kernel of DEFAULT_KERNELS) {
			out[kernel] = kernelMeasurement(kernel, focusSymbol, aggregate);
		}
		return out;
	}, [focusSymbol, tick]);

	return (
		<>
			{DEFAULT_KERNELS.map((kernel) => (
				<MetricRow
					key={kernel}
					name={kernel}
					measurement={measurements[kernel] ?? null}
				/>
			))}
		</>
	);
};
