import { useSelector } from "@tanstack/react-store";
import { balancesStore } from "#/collections/balances";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelSparkPaths,
	kernelStatusMeta,
} from "#/components/terminal/kernel-meta";
import {
	kernelFrameForSource,
	kernelHistoryCount,
	kernelHistoryValues,
	kernelReadingSource,
	kernelReadout,
	type ReadingsState,
} from "#/components/terminal/rows";

const timeMillis = (value: unknown): number | undefined => {
	const stamp =
		typeof value === "number" && Number.isFinite(value)
			? value
			: typeof value === "string" && value.trim() !== ""
				? Date.parse(value)
				: Number.NaN;

	return Number.isFinite(stamp) ? stamp : undefined;
};

const observedClock = (value: unknown): string => {
	const stamp = timeMillis(value);

	return stamp === undefined
		? "—"
		: new Date(stamp).toLocaleTimeString("en-US", { hour12: false });
};

const elapsedText = (
	value: unknown,
	now: number,
): { text: string; fresh: boolean } => {
	const stamp = timeMillis(value);

	if (stamp === undefined) {
		return { text: "— / —", fresh: false };
	}

	const ageMs = Math.max(0, now - stamp);
	const left =
		ageMs < 1000 ? `${Math.round(ageMs)}ms` : `${(ageMs / 1000).toFixed(1)}s`;

	return {
		text: `${left} / ${(ageMs / 1000).toFixed(1)}s`,
		fresh: ageMs < 2500,
	};
};

const finiteNumber = (value: unknown): number => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : 0;
};

const clamp01 = (value: number): number => Math.min(1, Math.max(0, value));

const heatColor = (value: number): { bg: string; fg: string } => {
	const v = clamp01(value);

	if (v >= 0.72) {
		return { bg: "#f2d197", fg: "#17140f" };
	}

	if (v >= 0.56) {
		return { bg: "#d9a13d", fg: "#17140f" };
	}

	if (v >= 0.4) {
		return { bg: "#46777d", fg: "var(--f2)" };
	}

	return { bg: "#23485b", fg: "var(--f3)" };
};

export type SignalDetailMeter = {
	color: string;
	label: string;
	percent: number;
	value: string;
};

export type SignalHeatmapCell = {
	bg: string;
	fg: string;
	key: string;
	label: string;
	title: string;
};

export const signalDetailModel = (
	readings: ReadingsState,
	selectedSource: string,
	focusSymbol: string,
	now = Date.now(),
) => {
	const frame = kernelFrameForSource(readings, selectedSource, focusSymbol);
	const { output, confidence, surprise, status, strength } =
		kernelReadout(frame);
	const source = kernelReadingSource(selectedSource);
	const bySymbol = readings[source] ?? {};
	const scopes = Object.keys(bySymbol).filter((scope) => scope.includes("/"));
	const active = scopes.filter((scope) => {
		const row = kernelReadout(bySymbol[scope]);

		return row.status !== "waiting" && row.status !== "standby";
	}).length;
	const copy = kernelCopy(
		selectedSource,
		String(output.category ?? selectedSource),
	);
	const classConfidence = finiteNumber(output.cognitiveClassConfidence);
	const observedAt =
		frame?.observed_at ?? output.observed_at ?? frame?.ts ?? output.ts;
	const elapsed = elapsedText(observedAt, now);
	const heatmap = scopes.slice(0, 24).map((scope) => {
		const row = kernelReadout(bySymbol[scope]);
		const color = heatColor(row.confidence);
		const base = scope.split("/")[0] ?? scope;

		return {
			bg: color.bg,
			fg: color.fg,
			key: scope,
			label: base,
			title: `${scope} · ${Math.floor(row.confidence * 100)}%`,
		};
	});

	return {
		activeText: `${active.toLocaleString()} / ${scopes.length.toLocaleString()}`,
		copy,
		gapText: String(output.gap ?? frame?.gap ?? "none"),
		heatmap,
		meters: [
			{
				label: "Confidence",
				value: `${Math.floor(confidence * 100)}%`,
				percent: confidence * 100,
				color: "var(--info)",
			},
			{
				label: "Surprise",
				value: `${surprise.toFixed(2)}×`,
				percent: Math.min(100, surprise * 100),
				color: status === "ambiguous" ? "var(--acc)" : "var(--info)",
			},
			{
				label: "Strength",
				value: strength.toFixed(4),
				percent: strength * 100,
				color: "var(--up)",
			},
			{
				label: "Class conf",
				value: `${Math.floor(classConfidence * 100)}%`,
				percent: classConfidence * 100,
				color: "var(--info)",
			},
		] satisfies SignalDetailMeter[],
		observedText: elapsed.text,
		status,
		strengthText: strength.toFixed(4),
	};
};

export const KernelInspector = () => {
	const inspectorSource = useSelector(
		terminalStore,
		(state) => state.inspectorSource,
	);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const readings = useSelector(measurementsStore, (state) => state);
	const { closeInspect, inspectSource } = terminalStore.actions;
	const source = inspectorSource ?? "";
	const frame =
		source === ""
			? undefined
			: kernelFrameForSource(readings, source, focusSymbol);
	const { output, confidence, surprise, status, strength } =
		kernelReadout(frame);
	const copy = kernelCopy(source, String(output.category ?? source));
	const statusMeta = kernelStatusMeta(status);
	const sparkValues = kernelHistoryValues(frame);
	const spark = kernelSparkPaths(sparkValues, status);
	const observed = observedClock(frame?.observed_at ?? output.observed_at);
	const samples = kernelHistoryCount(frame);

	if (inspectorSource === null || frame === undefined) {
		return null;
	}

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
						<div className="mb-1.5 flex items-center justify-between font-mono text-[9px] text-(--f4) uppercase tracking-[0.1em]">
							<span>signal history</span>
							<span>40 ticks</span>
						</div>
						<svg
							viewBox="0 0 150 30"
							preserveAspectRatio="none"
							className="block h-[52px] w-full rounded-[3px] border border-(--line) bg-(--sunken)"
						>
							<title>Signal history</title>
							<polyline points={spark.area} fill={spark.fill} stroke="none" />
							<polyline
								points={spark.spark}
								fill="none"
								stroke={spark.line}
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
								observed {observed} · {samples} samples
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

const InspectorMeter = ({
	label,
	value,
	percent,
	color,
}: {
	label: string;
	value: string;
	percent: number;
	color: string;
}) => (
	<div>
		<div className="mb-1 flex justify-between font-mono text-[10px]">
			<span className="text-(--f3)">{label}</span>
			<span className="text-(--f1)">{value}</span>
		</div>
		<div className="h-1 overflow-hidden rounded-[2px] bg-(--line)">
			<div
				className="h-full"
				style={{ width: `${percent}%`, background: color }}
			/>
		</div>
	</div>
);

export const SignalDetail = () => {
	const selectedSource = useSelector(
		terminalStore,
		(state) => state.selectedSource,
	);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const readings = useSelector(measurementsStore, (state) => state);
	const model = signalDetailModel(readings, selectedSource, focusSymbol);
	const statusMeta = kernelStatusMeta(model.status);

	return (
		<div className="min-h-0 overflow-auto px-5 py-[18px]">
			<div className="flex items-start justify-between gap-3">
				<div>
					<h2 className="font-serif font-semibold text-[24px] text-(--f1) leading-[1.1]">
						{model.copy.name}
					</h2>
					<div className="mt-1 font-mono text-[11px] text-(--f3)">
						{model.copy.sub}
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
				{model.copy.blurb}
			</p>
			<div className="mt-[18px] grid grid-cols-2 gap-x-[22px] gap-y-3">
				{model.meters.map((meter) => (
					<InspectorMeter
						key={meter.label}
						label={meter.label}
						value={meter.value}
						percent={meter.percent}
						color={meter.color}
					/>
				))}
			</div>
			<div className="mt-5 grid grid-cols-2 gap-x-[22px] gap-y-2 border-(--line) border-t pt-3.5 font-mono text-xs">
				<div className="flex justify-between">
					<span className="text-(--f3)">Active readings</span>
					<span className="text-(--f1)">{model.activeText}</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Strength</span>
					<span className="text-(--f1)">{model.strengthText}</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Observed</span>
					<span className="text-(--f1)">{model.observedText}</span>
				</div>
				<div className="flex justify-between">
					<span className="text-(--f3)">Gap</span>
					<span className="text-(--f1)">{model.gapText}</span>
				</div>
			</div>
			<div className="mt-[18px]">
				<div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Cross-section · confidence heatmap
				</div>
				{model.heatmap.length === 0 ? (
					<div className="py-6 text-center font-mono text-(--f4) text-xs">
						Waiting for cross-section readings
					</div>
				) : (
					<div className="grid grid-cols-12 gap-[3px]">
						{model.heatmap.map((cell) => (
							<div
								key={cell.key}
								title={cell.title}
								className="flex aspect-square items-center justify-center rounded-[2px] font-mono text-[8px]"
								style={{ backgroundColor: cell.bg, color: cell.fg }}
							>
								{cell.label}
							</div>
						))}
					</div>
				)}
			</div>
		</div>
	);
};

export const AllocationView = () => {
	const balances = useSelector(balancesStore, (state) => state.frame);
	const balancesList = (balances?.data as Array<Record<string, unknown>>) ?? [];
	const usdBalance =
		balancesList.find((b) => b.asset === "USD" || b.asset === "EUR") ??
		balancesList[0];
	const quoteCurrency = (usdBalance?.asset as string) || "USD";
	const readings = useSelector(measurementsStore, (state) => state);

	if (balancesList.length === 0) {
		return (
			<div className="min-h-0 flex-1 overflow-auto px-[18px] py-8 font-mono text-(--f4) text-xs text-center">
				Waiting for allocation balances...
			</div>
		);
	}

	return (
		<div className="min-h-0 flex-1 overflow-auto p-4 bg-(--sunken)">
			<table className="w-full font-mono text-[11px] text-(--f2) border-collapse">
				<thead>
					<tr className="border-b border-(--line) text-(--f4) text-left">
						<th className="pb-2 font-semibold uppercase tracking-wider">
							Asset
						</th>
						<th className="pb-2 font-semibold uppercase tracking-wider text-right">
							Balance
						</th>
						<th className="pb-2 font-semibold uppercase tracking-wider text-right">
							Price
						</th>
						<th className="pb-2 font-semibold uppercase tracking-wider text-right">
							Value
						</th>
					</tr>
				</thead>
				<tbody className="divide-y divide-(--line)">
					{balancesList.map((b) => {
						const asset = b.asset as string;
						const balance = Number(b.balance || 0);

						let price = 1;
						let displayPrice = "1.00";
						if (asset !== quoteCurrency) {
							const symbol = `${asset}/${quoteCurrency}`;
							let foundPrice = 0;
							for (const origin of Object.keys(readings)) {
								const frame = readings[origin]?.[symbol] as
									| Record<string, unknown>
									| undefined;
								if (frame?.price !== undefined) {
									foundPrice = Number(frame.price);
									break;
								}
								const output = frame?.output as
									| Record<string, unknown>
									| undefined;
								if (output?.last !== undefined) {
									foundPrice = Number(output.last);
									break;
								}
							}
							price = foundPrice;
							displayPrice = foundPrice > 0 ? foundPrice.toFixed(4) : "—";
						}

						const value = price > 0 ? balance * price : 0;
						const displayValue =
							value > 0 ? `${value.toFixed(2)} ${quoteCurrency}` : "—";

						return (
							<tr key={asset} className="hover:bg-(--surface)">
								<td className="py-2.5 font-semibold text-(--f1)">{asset}</td>
								<td className="py-2.5 text-right">{balance.toFixed(5)}</td>
								<td className="py-2.5 text-right text-(--f3)">
									{displayPrice}
								</td>
								<td className="py-2.5 text-right text-(--acc) font-semibold">
									{displayValue}
								</td>
							</tr>
						);
					})}
				</tbody>
			</table>
		</div>
	);
};
