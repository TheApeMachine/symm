import { useSelector } from "@tanstack/react-store";
import { useEffect, useMemo } from "react";
import { appStore } from "#/collections/app";
import {
	type MeasurementsState,
	measurementsStore,
} from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import { SignalDetail } from "#/components/kernel/detail";
import { HealthPanel, RadarPanel } from "#/components/terminal/health";
import { orderedKernelSources } from "#/components/terminal/kernel-meta";
import { KernelList } from "#/components/terminal/kernel-list";

export const signalsSurfaceSources = (
	kernels: string[],
	measurements: MeasurementsState,
): string[] =>
	orderedKernelSources([
		...new Set([...kernels, ...Object.keys(measurements.measurements)]),
	]);

export const SignalsSurface = () => {
	const kernels = useSelector(appStore, (state) => state.kernels);
	const measurements = useSelector(measurementsStore, (state) => state);
	const selectedSource = useSelector(
		terminalStore,
		(state) => state.selectedSource,
	);
	const sources = useMemo(
		() => signalsSurfaceSources(kernels, measurements),
		[kernels, measurements],
	);

	useEffect(() => {
		if (sources.length === 0 || sources.includes(selectedSource)) {
			return;
		}

		terminalStore.actions.selectSource(sources[0] ?? "");
	}, [selectedSource, sources]);

	return (
		<div className="grid h-full min-w-[1080px] grid-cols-[230px_minmax(420px,1fr)_320px]">
			<div className="min-h-0 overflow-auto border-(--line) border-r bg-(--surface)">
				<div className="sticky top-0 border-(--line) border-b bg-(--surface) px-3 py-2.5 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Kernels
				</div>
				<KernelList sources={sources} compact />
			</div>
			<div className="min-h-0 overflow-auto bg-(--bg)">
				<SignalDetail />
			</div>
			<div className="min-h-0 space-y-3.5 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
				<HealthPanel sources={sources} />
				<RadarPanel />
			</div>
		</div>
	);
};
