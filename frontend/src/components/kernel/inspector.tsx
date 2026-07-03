import { useSelector } from "@tanstack/react-store";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelStatusMeta,
	type SignalHealthStatus,
} from "#/components/terminal/kernel-meta";
import { InspectorMeter } from "./meter";

export const KernelInspector = () => {
	const inspectorSource = useSelector(
		terminalStore,
		(state) => state.inspectorSource,
	);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const measurements = useSelector(
		measurementsStore,
		(state) => state.measurements,
	);
	const { closeInspect, inspectSource } = terminalStore.actions;
	const source = inspectorSource ?? "";
	const history =
		source === ""
			? []
			: measurements[source]
					.values()
					.filter(
						(measurement) =>
							focusSymbol === "stream" ||
							measurement.scope === focusSymbol ||
							measurement.symbol === focusSymbol,
					);
	const frame = history.at(-1);
	const output =
		frame?.output !== null &&
		typeof frame?.output === "object" &&
		!Array.isArray(frame?.output)
			? (frame.output as Record<string, unknown>)
			: {};
	const confidence = Number(output.confidence ?? frame?.confidence ?? 0);
	const surprise = Number(output.surprise ?? frame?.surprise ?? 0);
	const strength = Number(output.strength ?? frame?.strength ?? confidence);
	const status: SignalHealthStatus =
		frame?.error !== undefined || output.error !== undefined
			? "fault"
			: confidence > 0.66
				? "measured"
				: confidence > 0
					? "ambiguous"
					: "standby";
	if (inspectorSource === null || frame === undefined) {
		return null;
	}

	const copy = kernelCopy(source, String(output.category ?? source));
	const statusMeta = kernelStatusMeta(status);
	const width = 150;
	const baseline = 29;
	const scale = 26;
	const values = history.slice(-40).flatMap((measurement) => {
		if (
			measurement.output === null ||
			typeof measurement.output !== "object" ||
			Array.isArray(measurement.output)
		) {
			return [];
		}

		const value = Number(
			(measurement.output as Record<string, unknown>).confidence,
		);

		return Number.isFinite(value) ? [value] : [];
	});
	const points = values
		.map(
			(value, index) =>
				`${((index / Math.max(values.length - 1, 1)) * width).toFixed(
					1,
				)},${(baseline - value * scale).toFixed(1)}`,
		)
		.join(" ");
	const observedAt = frame.observed_at ?? output.observed_at;
	const observed =
		typeof observedAt === "number" || typeof observedAt === "string"
			? new Date(observedAt).toLocaleTimeString("en-US", { hour12: false })
			: "—";

	return (
		<div className="absolute inset-y-0 left-[282px] right-[332px] z-9 animate-[symFade_0.18s_ease] bg-[color-mix(in_srgb,var(--sunken)_60%,transparent)] p-8 backdrop-blur-[3px]">
			<button
				type="button"
				aria-label="Close kernel inspector"
				className="absolute inset-0"
				onClick={closeInspect}
			/>
			<div className="pointer-events-none relative z-10 flex w-full items-start justify-center">
				<div className="pointer-events-auto w-full max-w-[452px] overflow-hidden rounded-[6px] border border-(--line2) bg-(--surface) shadow-[0_22px_54px_-14px_rgba(0,0,0,0.72)]">
					<div className="flex items-start justify-between gap-2.5 border-(--line) border-b px-4 pt-3.5 pb-[13px]">
						<div className="min-w-0">
							<div className="flex items-center gap-2">
								<span className="font-serif font-semibold text-[19px] text-(--f1) leading-[1.1]">
									{copy.name}
								</span>
								<span
									className="shrink-0 rounded-[2px] border px-1.5 py-0.5 text-[9px] font-semibold tracking-wider uppercase"
									style={{
										borderColor: statusMeta.bd,
										backgroundColor: statusMeta.bg,
										color: statusMeta.fg,
									}}
								>
									{statusMeta.label}
								</span>
							</div>
							<div className="mt-1 font-mono text-[10px] text-(--f4)">
								{copy.sub}
							</div>
						</div>
						<button
							type="button"
							onClick={closeInspect}
							className="flex size-[25px] shrink-0 cursor-pointer items-center justify-center rounded-[3px] border border-(--line) bg-(--raised) p-0 text-(--f3) hover:border-(--line2) hover:text-(--f1)"
						>
							<svg
								width="13"
								height="13"
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								strokeWidth="2"
								aria-hidden="true"
							>
								<path d="M6 6l12 12M18 6L6 18" />
							</svg>
						</button>
					</div>
					<p className="mx-4 mt-3.5 font-serif text-[14px] text-(--f2) leading-[1.56]">
						{copy.blurb}
					</p>
					<div className="mx-4 mt-3.5">
						<div className="mb-1.5 flex items-center justify-between font-mono text-[9px] text-(--f4) uppercase tracking-widest">
							<span>signal history</span>
							<span>40 ticks</span>
						</div>
						<svg
							viewBox="0 0 150 30"
							preserveAspectRatio="none"
							className="block h-[52px] w-full rounded-[3px] border border-(--line) bg-(--sunken)"
						>
							<title>Signal history</title>
							<polyline
								points={points}
								fill="none"
								stroke="var(--acc)"
								strokeWidth="1.5"
								vectorEffect="non-scaling-stroke"
							/>
						</svg>
					</div>
					<div className="flex flex-col gap-2.5 px-4 pt-3.5 pb-0.5">
						<InspectorMeter
							label="Confidence"
							value={`${Math.floor(confidence * 100)}%`}
							percent={confidence * 100}
							color="var(--info)"
						/>
						<InspectorMeter
							label="Surprise"
							value={surprise.toFixed(2)}
							percent={Math.min(100, surprise * 100)}
							color="var(--acc)"
						/>
						<InspectorMeter
							label="Strength"
							value={strength.toFixed(4)}
							percent={Math.min(100, strength * 100)}
							color="var(--up)"
						/>
					</div>
					<div className="mt-[11px] flex items-center justify-between gap-3 border-(--line) border-t bg-(--sunken) px-4 py-3.5">
						<div className="min-w-0 font-mono text-[9.5px] text-(--f4) leading-[1.55]">
							<div>active {String(frame.scope ?? focusSymbol)}</div>
							<div>
								observed {observed} · {history.length} samples
							</div>
						</div>
						<button
							type="button"
							onClick={() => inspectSource(inspectorSource)}
							className="shrink-0 cursor-pointer rounded-[3px] border border-[color-mix(in_srgb,var(--acc)_45%,transparent)] bg-[color-mix(in_srgb,var(--acc)_12%,transparent)] px-3 py-2 font-semibold text-[11px] text-(--acc) whitespace-nowrap hover:bg-[color-mix(in_srgb,var(--acc)_22%,transparent)]"
						>
							Open in signal insight →
						</button>
					</div>
				</div>
			</div>
		</div>
	);
};
