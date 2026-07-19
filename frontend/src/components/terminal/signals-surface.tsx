import { useSelector } from "@tanstack/react-store";
import { useEffect, useMemo, useState } from "react";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import type { Measurement } from "#/types/measurement";
import { SignalDetail } from "#/components/kernel/detail";
import { CrossSectionPanel } from "#/components/terminal/cross-section-panel";
import { HealthPanel } from "#/components/terminal/health";
import { KernelList } from "#/components/terminal/kernel-list";
import { orderedKernelSources } from "#/components/terminal/kernel-meta";
import { backendMeasurementSources } from "#/components/terminal/measurement-sources";
import { RadarPanel } from "#/components/terminal/regime-radar";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { getWorker } from "#/providers/websocket";

export const signalsSurfaceSources = (
	kernels: string[],
	backendSources: string[],
): string[] =>
	orderedKernelSources([...new Set([...kernels, ...backendSources])]);

/*
SignalsSurface keeps React reconciliation scoped to source-key changes while
every live readout inside the surface bypasses React via direct store paint.
*/
export const SignalsSurface = () => {
	const kernels = useSelector(appStore, (state) => state.kernels);
	const online = useSelector(appStore, (state) => state.online);
	const selectedSource = useSelector(
		terminalStore,
		(state) => state.selectedSource,
	);
	const [backendSources, setBackendSources] = useState<string[]>([]);

	useDirectStorePaint(
		getWorker(),
		[{ store: "measurements", key: "" }],
		(buffers) => {
			const next = backendMeasurementSources(
				(buffers["measurements:"] ?? []) as Measurement[],
			);

			setBackendSources((previous) =>
				previous.length === next.length &&
				previous.every((source, index) => source === next[index])
					? previous
					: next,
			);
		},
		[online],
	);

	const sources = useMemo(
		() => signalsSurfaceSources(kernels, backendSources),
		[kernels, backendSources],
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
				<CrossSectionPanel />
				<RadarPanel sources={sources} />
			</div>
		</div>
	);
};
