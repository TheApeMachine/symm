import { useSelector } from "@tanstack/react-store";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import { kernelCopy } from "#/components/terminal/kernel-meta";

export const KernelInspector = () => {
	const inspectorSource = useSelector(
		terminalStore,
		(state) => state.inspectorSource,
	);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const readings = useSelector(measurementsStore, (state) => state.readings);
	const { closeInspect, inspectSource } = terminalStore.actions;

	if (inspectorSource === null) {
		return null;
	}

	const frame = readings[inspectorSource]?.[focusSymbol];

	if (frame === undefined) {
		return null;
	}

	const output = (frame.output ?? {}) as Record<string, unknown>;
	const confidence = (output.confidence as number) ?? 0;
	const surprise = (output.surprise as number) ?? 0;
	const strength = (output.strength as number) ?? 0;
	const copy = kernelCopy(
		inspectorSource,
		String(output.category ?? inspectorSource),
	);

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
									{inspectorSource}
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
					<div className="flex flex-col gap-2.5 px-4 pt-3.5 pb-0.5">
						<InspectorMeter
							label="Confidence"
							value={`${Math.round(confidence * 100)}%`}
							percent={Math.round(confidence * 100)}
							color="var(--info)"
						/>
						<InspectorMeter
							label="Surprise"
							value={surprise.toFixed(2)}
							percent={Math.min(100, surprise * 50)}
							color="var(--acc)"
						/>
						<InspectorMeter
							label="Strength"
							value={strength.toFixed(4)}
							percent={Math.round(Math.min(100, strength * 100))}
							color="var(--up)"
						/>
					</div>
					<div className="mt-[11px] flex items-center justify-between gap-3 border-(--line) border-t bg-(--sunken) px-4 py-3.5">
						<div className="min-w-0 font-mono text-[9.5px] text-(--f4) leading-[1.55]">
							<div>scope {String(frame.scope ?? "—")}</div>
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
	const readings = useSelector(measurementsStore, (state) => state.readings);
	const frame = readings[selectedSource]?.[focusSymbol];

	if (frame === undefined) {
		return (
			<div className="p-5 font-mono text-(--f4) text-xs">
				Waiting for signal readings
			</div>
		);
	}

	const output = (frame.output ?? {}) as Record<string, unknown>;
	const confidence = (output.confidence as number) ?? 0;
	const surprise = (output.surprise as number) ?? 0;
	const strength = (output.strength as number) ?? 0;
	const copy = kernelCopy(
		selectedSource,
		String(output.category ?? selectedSource),
	);

	return (
		<div className="min-h-0 overflow-auto px-5 py-[18px]">
			<div className="flex items-start justify-between gap-3">
				<div>
					<h2 className="font-serif font-semibold text-[24px] text-(--f1) leading-[1.1]">
						{selectedSource}
					</h2>
					<div className="mt-1 font-mono text-[11px] text-(--f3)">
						{copy.sub}
					</div>
				</div>
			</div>
			<p className="mt-3.5 max-w-[560px] font-serif text-[15px] text-(--f2) leading-[1.55]">
				{copy.blurb}
			</p>
			<div className="mt-[18px] grid grid-cols-2 gap-x-[22px] gap-y-3">
				<InspectorMeter
					label="Confidence"
					value={`${Math.round(confidence * 100)}%`}
					percent={Math.round(confidence * 100)}
					color="var(--info)"
				/>
				<InspectorMeter
					label="Surprise"
					value={surprise.toFixed(2)}
					percent={Math.min(100, surprise * 50)}
					color="var(--acc)"
				/>
				<InspectorMeter
					label="Strength"
					value={strength.toFixed(4)}
					percent={Math.round(Math.min(100, strength * 100))}
					color="var(--up)"
				/>
			</div>
		</div>
	);
};

export const AllocationView = () => (
	<div className="min-h-0 overflow-auto px-[18px] py-4 font-mono text-(--f4) text-xs">
		waiting for allocation frames
	</div>
);
