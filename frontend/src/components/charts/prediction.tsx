import { useSelector } from "@tanstack/react-store";
import { focusStore, resonanceStore } from "#/collections/app";
import { Resonance } from "#/providers/telemetry/telemetry/resonance";
import { ResonanceForecast } from "#/providers/telemetry/telemetry/resonance-forecast";

export const vectorSlotTransform = (slot: number, slotCount: number): string =>
	`translateX(${(slot / slotCount) * 100}%) scaleX(${1 / slotCount})`;

export const signedVectorTransform = "scaleY(calc(var(--value, 0) * -1))";

const fmt = (value: number | undefined | null, digits: number): string =>
	value === undefined || value === null || !Number.isFinite(value) ? "—" : value.toFixed(digits);

const dir = (value: number | undefined | null): string => {
	if (value === undefined || value === null) return "—";
	if (value > 0) return "up";
	if (value < 0) return "down";
	return "flat";
};

const resObj = new Resonance();
const forecastObj = new ResonanceForecast();

const ScalarDiagnostics = () => {
	const symbol = useSelector(focusStore, (state) => state);
	const frameWithSymbol = useSelector(resonanceStore, (state) =>
		state.findLast((frame) => {
			for (let i = 0; i < frame.rowsLength(); i++) {
				const row = frame.rows(i, resObj);
				if (row && row.symbol() === symbol) {
					return true;
				}
			}
			return false;
		}),
	);

	let res: Resonance | null = null;
	if (frameWithSymbol) {
		for (let i = 0; i < frameWithSymbol.rowsLength(); i++) {
			const row = frameWithSymbol.rows(i, resObj);
			if (row && row.symbol() === symbol) {
				res = row;
				break;
			}
		}
	}

	const fcast = res ? res.forecast(forecastObj) : null;

	return (
		<div className="grid grid-cols-5 gap-px overflow-hidden border border-(--line) bg-(--line)">
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">relative precision</div>
				<div data-p="prec" className="mt-0.5 font-mono text-[11px] text-(--up)">
					{res ? fmt(res.taskRelativePrecision(), 3) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">task skill</div>
				<div data-p="skill" className="mt-0.5 font-mono text-[11px] text-(--f2)">
					{res ? fmt(res.taskSkill(), 3) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">issued t</div>
				<div data-p="issued" className="mt-0.5 font-mono text-[11px] text-(--f2)">
					{res ? dir(res.lastResolvedForecast()) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">realized t+1</div>
				<div data-p="realized" className="mt-0.5 font-mono text-[11px] text-(--f2)">
					{res ? dir(res.lastRealizedReturn()) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">forecast error</div>
				<div data-p="error" className="mt-0.5 font-mono text-[11px] text-(--f2)">
					{res ? fmt(res.lastForecastError(), 0) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">horizon / reach</div>
				<div className="mt-0.5 flex gap-1 font-mono text-[11px] text-(--f2)">
					<span data-p="horizon">{fcast ? fmt(Number(fcast.supportedHorizon()), 0) : "—"}</span>
					<span>/</span>
					<span data-p="reach">{fcast ? fmt(Number(fcast.probeHorizon()), 0) : "—"}</span>
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">resolved samples</div>
				<div data-p="samples" className="mt-0.5 font-mono text-[11px] text-(--acc)">
					{res ? String(res.samples()) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">surprise</div>
				<div data-p="surprise" className="mt-0.5 truncate font-mono text-[11px] text-(--warning)">
					{res ? fmt(res.surprise(), 2) : "—"}
				</div>
			</div>
			<div className="bg-[#0a0907] px-2 py-1.5">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">energy</div>
				<div data-p="energy" className="mt-0.5 truncate font-mono text-[11px] text-(--info)">
					{res ? fmt(res.energy(), 2) : "—"}
				</div>
			</div>
		</div>
	);
};

const VerdictRow = () => {
	const symbol = useSelector(focusStore, (state) => state);
	const frameWithSymbol = useSelector(resonanceStore, (state) =>
		state.findLast((frame) => {
			for (let i = 0; i < frame.rowsLength(); i++) {
				const row = frame.rows(i, resObj);
				if (row && row.symbol() === symbol) {
					return true;
				}
			}
			return false;
		}),
	);

	let res: Resonance | null = null;
	if (frameWithSymbol) {
		for (let i = 0; i < frameWithSymbol.rowsLength(); i++) {
			const row = frameWithSymbol.rows(i, resObj);
			if (row && row.symbol() === symbol) {
				res = row;
				break;
			}
		}
	}

	return (
		<div className="grid grid-cols-3 gap-px border border-(--line) bg-(--line)">
			<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">residual model</div>
				<div className="flex items-baseline gap-2">
					<span className="size-1.5 shrink-0 self-center rounded-full bg-(--acc)" />
					<span data-p="calibration" className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)">
						{res ? String(res.taskCalibration() ?? "—") : "—"}
					</span>
				</div>
			</div>
			<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">direction skill</div>
				<div className="flex items-baseline gap-2">
					<span className="size-1.5 shrink-0 self-center rounded-full bg-(--acc)" />
					<span data-p="skillStatus" className="truncate font-mono text-[13px] uppercase tracking-wide text-(--f2)">
						{res ? String(res.taskSkillStatus() ?? "—") : "—"}
					</span>
				</div>
			</div>
			<div className="flex flex-col justify-between gap-1.5 bg-[#0a0907] px-3 py-2">
				<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">forecast</div>
				<div className="flex items-center gap-2">
					<span className="inline-block shrink-0 text-[15px] leading-none text-(--acc)">▶</span>
					<span data-p="forecast" className="truncate font-mono text-[13px] text-(--acc)">
						{res ? dir(res.taskForecast()) : "—"}
					</span>
				</div>
			</div>
		</div>
	);
};

export const TerminalPredictionChart = () => (
	<div className="flex size-full flex-col gap-3 px-4 pt-14 pb-3">
		<VerdictRow />
		<ScalarDiagnostics />
	</div>
);