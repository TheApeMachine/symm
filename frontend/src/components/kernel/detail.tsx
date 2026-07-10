import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelStatusMeta,
	type SignalHealthStatus,
} from "#/components/terminal/kernel-meta";
import { InspectorMeter } from "./meter";

const clampPercent = (value: number) => Math.max(0, Math.min(100, value * 100));

const finite = (value: unknown): number => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : 0;
};

const heatColor = (value: number): [number, number, number] => {
	const stops: Array<[number, [number, number, number]]> = [
		[0, [14, 12, 10]],
		[0.4, [26, 34, 50]],
		[0.6, [42, 106, 129]],
		[0.8, [232, 163, 61]],
		[1, [246, 214, 159]],
	];
	const t = Math.max(0, Math.min(1, value));

	for (let index = 0; index < stops.length - 1; index += 1) {
		const left = stops[index];
		const right = stops[index + 1];

		if (t <= right[0]) {
			const span = right[0] - left[0];
			const mix = span === 0 ? 0 : (t - left[0]) / span;

			return [
				left[1][0] + (right[1][0] - left[1][0]) * mix,
				left[1][1] + (right[1][1] - left[1][1]) * mix,
				left[1][2] + (right[1][2] - left[1][2]) * mix,
			];
		}
	}

	return stops[stops.length - 1][1];
};

const stampOf = (frame: { at?: string | number } | undefined): number => {
	const at = frame?.at;

	if (typeof at === "number") {
		return at;
	}

	if (typeof at === "string" && at.trim() !== "") {
		return Date.parse(at);
	}

	return Number.NaN;
};

const ageText = (stamp: number): string => {
	if (!Number.isFinite(stamp)) {
		return "—";
	}

	const age = Math.max(0, Date.now() - stamp);

	if (age < 1000) {
		return `${Math.round(age)}ms`;
	}

	return `${(age / 1000).toFixed(1)}s`;
};

export const SignalDetail = () => {
	const selectedSource = useSelector(
		terminalStore,
		(state) => state.selectedSource,
	);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const measurements = useSelector(measurementsStore, (state) => state);
	const source = selectedSource;
	const history =
		measurements.measurements[focusSymbol]?.[source]?.values() ?? [];
	const measurement = history.at(-1);
	const category = measurement?.categories.at(0);
	const metrics = measurement?.metrics ?? {};
	const confidence = finite(category?.confidence);
	const surprise = finite(category?.surprisal);
	const strength = finite(category?.strength);
	const backendStatus = measurement?.status ?? "";
	const status = (
		measurement === undefined
			? "waiting"
			: backendStatus === "fault" ||
					backendStatus === "ambiguous" ||
					backendStatus === "calibrating"
				? backendStatus
				: "measured"
	) as SignalHealthStatus;
	const categoryType =
		typeof category?.type === "string" ? category.type : selectedSource;
	const copy = kernelCopy(selectedSource, categoryType);
	const statusMeta = kernelStatusMeta(status);
	const observedStamp = stampOf(measurement);
	const active = Object.values(measurements.measurements).reduce(
		(sum, sources) => sum + (sources[source]?.values().length ?? 0),
		0,
	);
	const total = Object.values(measurements.measurements).reduce(
		(sum, sources) =>
			sum +
			Object.values(sources).reduce(
				(sourceSum, sourceHistory) => sourceSum + sourceHistory.values().length,
				0,
			),
		0,
	);
	const heatmap = Object.entries(measurements.measurements).flatMap(
		([symbol, sources]) => {
			const frame = sources[source]?.values().at(-1);
			const value = finite(frame?.categories.at(0)?.confidence);

			return frame === undefined ? [] : [{ symbol, value }];
		},
	);

	return (
		<div className="min-h-0 overflow-auto px-5 py-[18px]">
			<div className="flex items-start justify-between gap-3">
				<div>
					<h2 className="font-serif font-semibold text-[24px] text-(--f1) leading-[1.1]">
						{copy.name}
					</h2>
					<div className="mt-1 font-mono text-[11px] text-(--f3)">
						{copy.sub}
					</div>
				</div>
				<span
					className="shrink-0 rounded-[2px] border px-2.5 py-1 text-[11px] font-semibold tracking-wider uppercase"
					style={{
						borderColor: statusMeta.bd,
						backgroundColor: statusMeta.bg,
						color: statusMeta.fg,
					}}
				>
					{statusMeta.label}
				</span>
			</div>
			<p className="mt-3.5 max-w-[560px] font-serif text-[15px] text-(--f2) leading-[1.55]">
				{copy.blurb}
			</p>
			{measurement === undefined ? (
				<div className="mt-[18px] rounded border border-(--line) bg-(--sunken) px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
					waiting for backend {selectedSource} measurement
				</div>
			) : null}
			{measurement === undefined ? null : (
				<div className="mt-[18px] grid grid-cols-2 gap-x-[22px] gap-y-3">
					<InspectorMeter
						label="Confidence"
						value={`${Math.floor(confidence * 100)}%`}
						percent={confidence * 100}
						color="var(--info)"
					/>
					<InspectorMeter
						label="Surprise"
						value={`${surprise.toFixed(2)}x`}
						percent={clampPercent(surprise)}
						color={surprise >= 1 ? "var(--acc)" : "var(--info)"}
					/>
					<InspectorMeter
						label="Strength"
						value={strength.toFixed(4)}
						percent={clampPercent(strength)}
						color="var(--up)"
					/>
					<InspectorMeter
						label="Evidence"
						value={measurement === undefined ? "Missing" : "Ready"}
						percent={measurement === undefined ? 0 : 100}
						color="var(--up)"
					/>
					<InspectorMeter
						label="Freshness"
						value={ageText(observedStamp)}
						percent={measurement === undefined ? 0 : 100}
						color="var(--info)"
					/>
					<InspectorMeter
						label="Calibration"
						value={status === "measured" ? "Ready" : statusMeta.label}
						percent={status === "measured" ? 100 : 0}
						color={status === "measured" ? "var(--up)" : statusMeta.fg}
					/>
				</div>
			)}
			<div className="mt-5 grid grid-cols-2 gap-x-[22px] gap-y-2 border-(--line) border-t pt-3.5 font-mono text-xs">
				<div className="flex justify-between">
					<span className="text-(--f3)">Active readings</span>
					<span className="text-(--f1)">
						{active.toLocaleString()} / {total.toLocaleString()}
					</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Strength</span>
					<span className="text-(--f1)">{strength.toFixed(4)}</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Observed</span>
					<span className="text-(--f1)">
						{Number.isFinite(observedStamp)
							? `${new Date(observedStamp).toLocaleTimeString("en-US", {
									hour12: false,
								})} / ${ageText(observedStamp)}`
							: "— / —"}
					</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Gap</span>
					<span className="text-(--f1)">{String(metrics.gap ?? "none")}</span>
				</div>
			</div>
			<div className="mt-[18px]">
				<div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Cross-section · confidence heatmap
				</div>
				<div className="grid grid-cols-12 gap-[3px]">
					{heatmap.map((cell) => {
						const percent = Math.round(clampPercent(cell.value));
						const label = cell.symbol.split("/")[0] ?? cell.symbol;
						const [red, green, blue] = heatColor(cell.value);

						return (
							<div
								key={cell.symbol}
								title={`${cell.symbol} · ${percent}%`}
								className="flex aspect-square items-center justify-center rounded-[2px] font-mono text-[8px]"
								style={{
									background: `rgb(${Math.round(red)}, ${Math.round(
										green,
									)}, ${Math.round(blue)})`,
									color: cell.value > 0.62 ? "#14110f" : "var(--f3)",
								}}
							>
								{label}
							</div>
						);
					})}
				</div>
			</div>
		</div>
	);
};
