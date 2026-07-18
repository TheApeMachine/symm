import { useSelector } from "@tanstack/react-store";
import {
	flattenMeasurementBuffer,
	measurementsStore,
} from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import { requireSampleSize } from "#/lib/domain";

export const KernelRow = ({ source }: { source: string }) => {
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const history = useSelector(measurementsStore, (state) =>
		flattenMeasurementBuffer(state.measurements[focusSymbol]?.[source]),
	);
	const measurement = history.at(-1);
	const confidence = measurement?.categories?.at(0)?.confidence ?? 0;
	const points =
		history.length === 0
			? ""
			: history.length === 1
				? `0,${(1 - confidence).toFixed(3)} 1,${(1 - confidence).toFixed(3)}`
				: (() => {
						requireSampleSize(history.length, 2, "kernel row sparkline");
						const span = history.length - 1;

						return history
							.map(
								(item, index) =>
									`${(index / span).toFixed(3)},${(
										1 - (item.categories?.at(0)?.confidence ?? 0)
									).toFixed(3)}`,
							)
							.join(" ");
					})();

	return (
		<button
			type="button"
			onClick={() => terminalStore.actions.inspectSource(source)}
			className="block w-full cursor-pointer px-4 py-3 text-left hover:bg-(--raised)"
		>
			<div className="flex items-center justify-between gap-3">
				<div className="min-w-0 flex-1">
					<div className="truncate font-serif font-semibold text-[15px] text-(--f1)">
						{source}
					</div>
					<div className="mt-2">
						<svg
							viewBox="0 0 1 1"
							preserveAspectRatio="none"
							className="h-5 w-full overflow-visible"
						>
							<title>{`${source} confidence`}</title>
							<line
								x1="0"
								y1="1"
								x2="1"
								y2="1"
								stroke="var(--line)"
								strokeWidth="1.2"
								vectorEffect="non-scaling-stroke"
							/>
							{points === "" ? null : (
								<polyline
									points={points}
									fill="none"
									stroke="var(--acc)"
									strokeWidth="1.4"
									vectorEffect="non-scaling-stroke"
								/>
							)}
						</svg>
						<svg
							viewBox="0 0 1 1"
							preserveAspectRatio="none"
							className="mt-1 h-1 w-full bg-(--line)"
						>
							<title>{`${source} confidence meter`}</title>
							<rect
								x="0"
								y="0"
								width={measurement === undefined ? 0 : confidence}
								height="1"
								fill="var(--acc)"
							/>
						</svg>
					</div>
				</div>
				<div className="shrink-0 text-right font-mono text-[10px]">
					<div
						className={
							measurement === undefined ? "text-(--f4)" : "text-(--f2)"
						}
					>
						{measurement === undefined
							? "waiting"
							: `${(confidence * 100).toFixed(0)}%`}
					</div>
					{measurement === undefined ? null : (
						<div className="text-(--f4)">{measurement.symbol}</div>
					)}
				</div>
			</div>
		</button>
	);
};
