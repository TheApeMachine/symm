import { useSelector } from "@tanstack/react-store";
import { useMemo, useRef } from "react";
import { atoms, DEFAULT_KERNELS, signals } from "#/collections/app";
import type { Value } from "#/types/value";
import { MetricRow, stickyKernelMeasurements } from "./metric";

/*
One workspace subscription per dashboard column: each kernel row still binds
through Component, but React reconciles one parent per tick instead of ten.
*/
export const KernelMetricStrip = () => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const aggregate = useSelector(
		signals.workspace,
		(state) => state.aggregate,
	);

	const stickyRef = useRef<Record<string, Record<string, Value>>>({});

	const measurements = useMemo(
		() =>
			stickyKernelMeasurements(
				stickyRef.current,
				DEFAULT_KERNELS,
				focusSymbol,
				aggregate,
			),
		[focusSymbol, aggregate],
	);

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
