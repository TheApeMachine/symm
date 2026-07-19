import { useSelector } from "@tanstack/react-store";
import { useState } from "react";
import { appStore } from "#/collections/app";
import type { Measurement } from "#/types/measurement";
import { KernelListRow } from "#/components/terminal/kernel-list-row";
import { backendMeasurementSources } from "#/components/terminal/measurement-sources";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { getWorker } from "#/providers/websocket";

/*
KernelList renders one direct-painted row per backend source. Source keys come
from worker measurement snapshots so tick cadence never re-renders from stores.
*/
export const KernelList = ({
	compact = false,
	sources: inputSources,
}: {
	compact?: boolean;
	sources?: string[];
}) => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const online = useSelector(appStore, (state) => state.online);
	const [storeSources, setStoreSources] = useState<string[]>([]);

	useDirectStorePaint(
		getWorker(),
		[{ store: "measurements", key: "" }],
		(buffers) => {
			const next = backendMeasurementSources(
				(buffers["measurements:"] ?? []) as Measurement[],
			);

			setStoreSources((previous) =>
				previous.length === next.length &&
				previous.every((source, index) => source === next[index])
					? previous
					: next,
			);
		},
		[online],
	);

	const sources = inputSources ?? storeSources;

	if (sources.length === 0) {
		return (
			<div className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)">
				waiting for backend measurement frames
			</div>
		);
	}

	return (
		<div className="min-h-0 overflow-auto">
			{sources.map((source) => (
				<KernelListRow
					key={source}
					source={source}
					focusSymbol={focusSymbol}
					compact={compact}
				/>
			))}
		</div>
	);
};
