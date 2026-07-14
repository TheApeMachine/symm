import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import { KernelListRow } from "#/components/terminal/kernel-list-row";
import {
	backendMeasurementSources,
	sameSources,
} from "#/components/terminal/measurement-sources";

/*
KernelList renders one direct-painted row per backend source. The parent only
subscribes to source-key changes so tick cadence never re-renders the list shell.
*/
export const KernelList = ({
	compact = false,
	sources: inputSources,
}: {
	compact?: boolean;
	sources?: string[];
}) => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const storeSources = useSelector(
		measurementsStore,
		(state) =>
			backendMeasurementSources({
				measurements: {
					[focusSymbol]: state.measurements[focusSymbol] ?? {},
				},
			}),
		{ compare: sameSources },
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
