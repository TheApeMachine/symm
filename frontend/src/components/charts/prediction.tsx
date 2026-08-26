import { useLayoutEffect, useState } from "react";
import { resonanceFocusStore, useSubscribe } from "#/providers/ws-stores";

export const vectorSlotTransform = (slot: number, slotCount: number): string =>
	`translateX(${(slot / slotCount) * 100}%) scaleX(${1 / slotCount})`;

export const signedVectorTransform = "scaleY(calc(var(--value, 0) * -1))";

const fmt = (value: number | undefined, digits: number): string =>
	value === undefined ? "" : value.toFixed(digits);

const dir = (value: number | undefined): string => {
	if (value === undefined) return "";
	if (value > 0) return "up";
	if (value < 0) return "down";
	return "flat";
};

const ScalarDiagnostics = () => {
	const root = useSubscribe(resonanceFocusStore, (state) => {
		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-p="${q}"]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		set("prec", fmt(state?.taskRelativePrecision, 3));
		set("skill", fmt(state?.taskSkill, 3));
		set("issued", dir(state?.lastResolvedForecast));
		set("realized", dir(state?.lastRealizedReturn));
		set("error", fmt(state?.lastForecastError, 0));
		set("horizon", fmt(state?.forecast?.supportedHorizon, 0));
		set("reach", fmt(state?.forecast?.probeHorizon, 0));
		set("samples", fmt(state?.samples, 0));
		set("surprise", fmt(state?.surprise, 2));
		set("energy", fmt(state?.energy, 2));
	});

	return (
		<div ref={root} className="grid grid-cols-5 gap-px overflow-hidden border border-(--line) bg-(--line)">
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">relative precision</div>
				<div data-p="prec" className="mt-0.5 font-mono text-[11px] text-(--up)">—</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">task skill</div>
				<div data-p="skill" className="mt-0.5 font-mono text-[11px] text-(--f2)">—</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">issued t</div>
				<div data-p="issued" className="mt-0.5 font-mono text-[11px] text-(--f2)">—</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">realized t+1</div>
				<div data-p="realized" className="mt-0.5 font-mono text-[11px] text-(--f2)">—</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">forecast error</div>
				<div data-p="error" className="mt-0.5 font-mono text-[11px] text-(--f2)">—</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">horizon / reach</div>
				<div className="mt-0.5 flex gap-1 font-mono text-[11px] text-(--f2)">
					<span data-p="horizon">—</span>
					<span>/</span>
					<span data-p="reach">—</span>
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">resolved samples</div>
				<div data-p="samples" className="mt-0.5 font-mono text-[11px] text-(--acc)">—</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">surprise</div>
				<div data-p="surprise" className="mt-0.5 truncate font-mono text-[11px] text-(--warning)">—</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">energy</div>
				<div data-p="energy" className="mt-0.5 truncate font-mono text-[11px] text-(--info)">—</div>
			</div>
		</div>
	);
};

const VerdictRow = () => {
	const root = useSubscribe(resonanceFocusStore, (state) => {
		const setVerdict = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-p="${q}"]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		setVerdict("calibration", String(state?.taskCalibration ?? "—"));
		setVerdict("skillStatus", String(state?.taskSkillStatus ?? "—"));
		setVerdict("forecast", dir(state?.taskForecast));
	});

	return (
		<div ref={root} className="grid grid-cols-3 gap-px border border-(--line) bg-(--line)">
			<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">residual model</div>
				<div className="flex items-baseline gap-2">
					<span className="size-1.5 shrink-0 self-center rounded-full bg-(--acc)" />
					<span data-p="calibration" className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)">—</span>
				</div>
			</div>
			<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">direction skill</div>
				<div className="flex items-baseline gap-2">
					<span className="size-1.5 shrink-0 self-center rounded-full bg-(--acc)" />
					<span data-p="skillStatus" className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)">—</span>
				</div>
			</div>
			<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">forecast</div>
				<div className="flex items-center gap-2">
					<span className="inline-block shrink-0 text-[15px] leading-none text-(--acc)">▶</span>
					<span data-p="forecast" className="truncate font-mono text-[13px] text-(--acc)">—</span>
				</div>
			</div>
		</div>
	);
};

const VectorLane = ({
	label,
	meta,
	color,
	read,
}: {
	label: string;
	meta: string;
	color: string;
	read: (state: ReturnType<typeof resonanceFocusStore.get>) => number[] | undefined;
}) => {
	const [slotCount, setSlotCount] = useState(() => read(resonanceFocusStore.state)?.length ?? 0);

	useLayoutEffect(() => {
		const apply = (state: ReturnType<typeof resonanceFocusStore.get>) => {
			const next = read(state)?.length ?? 0;
			setSlotCount((current) => (current === next ? current : next));
		};

		apply(resonanceFocusStore.state);
		const subscription = resonanceFocusStore.subscribe(apply);

		return () => subscription.unsubscribe();
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [read]);

	const root = useSubscribe(resonanceFocusStore, (state) => {
		const values = read(state) ?? [];

		const slots = root.current?.querySelectorAll<HTMLElement>("[data-slot]");

		slots?.forEach((slot, index) => {
			const value = values[index] ?? 0;
			slot.style.setProperty("--value", String(value));
			slot.title = value.toFixed(4);
		});
	});

	return (
		<div ref={root} className="flex min-h-0 flex-1 items-stretch gap-3">
			<div className="flex w-36 shrink-0 flex-col justify-center gap-0.5 font-mono text-[9px] leading-tight">
				<span className="font-semibold uppercase tracking-widest text-(--f3)">{label}</span>
				<span className="text-(--f4)">{meta}</span>
			</div>
			<div className="relative min-h-0 flex-1 overflow-hidden border border-(--line) bg-[linear-gradient(to_bottom,transparent_calc(50%-0.5px),var(--line2)_calc(50%-0.5px),var(--line2)_calc(50%+0.5px),transparent_calc(50%+0.5px))]">
				{Array.from({ length: slotCount }, (_, slot) => (
					<div
						// biome-ignore lint/suspicious/noArrayIndexKey: one fixed-geometry slot per vector component; the index is the slot's identity and the vector never reorders.
						key={`slot-${slot}`}
						data-slot
						className="absolute inset-y-0 right-1 left-1 origin-left"
						style={{ transform: vectorSlotTransform(slot, slotCount) }}
					>
						<div
							className={`absolute top-1/2 right-1.5 left-1 h-[calc(50%-1px)] origin-top ${color}`}
							style={{ transform: signedVectorTransform }}
						/>
					</div>
				))}
			</div>
		</div>
	);
};

const HIERARCHY = [
	{ index: 0, label: "L0 · sensory", meta: "bottom-up reconstruction" },
	{ index: 1, label: "L1 · micro", meta: "adjacent generative link" },
	{ index: 2, label: "L2 · macro", meta: "context state" },
] as const;

export const TerminalPredictionChart = () => (
	<div className="flex size-full flex-col gap-3 px-4 pt-14 pb-3">
		<VerdictRow />
		<ScalarDiagnostics />
		{HIERARCHY.map((layer) => (
			<VectorLane
				key={layer.label}
				label={layer.label}
				meta={layer.meta}
				color="bg-(--f3)"
				read={(state) => state?.layers?.[layer.index]?.state}
			/>
		))}
		<VectorLane
			label="Latent state z"
			meta="settled predictive state · zero centered"
			color="bg-(--info)"
			read={(state) => state?.latent}
		/>
		<VectorLane
			label="Forward direction shape"
			meta="signed direction lean · t+1 → t+k"
			color="bg-(--acc)"
			read={(state) => state?.forecast?.forwardCurve}
		/>
	</div>
);