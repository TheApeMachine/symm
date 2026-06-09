import { CircleAlertIcon } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";
import {
	drawSignalGauge,
	type SignalGaugeControls,
} from "#/components/charts/confidence/draw-signal-gauge";
import {
	confidenceFromGaugePayload,
	gaugePayloadEntries,
	gaugeWarmupPercent,
	surpriseFromGaugePayload,
	surpriseScaleMax,
	surpriseThresholdFromGaugePayload,
} from "#/components/charts/confidence/gauge-payload";
import { Card, CardPanel } from "#/components/ui/card";
import { Frame, FrameFooter } from "#/components/ui/frame";
import {
	Progress,
	ProgressIndicator,
	ProgressLabel,
	ProgressTrack,
	ProgressValue,
} from "#/components/ui/progress";

export type SignalGaugeBridge = {
	update: (wire: Record<string, unknown>) => void;
	ready: boolean;
	pending: Record<string, unknown>[];
	latest: Record<string, unknown>;
};

// Keys rendered by the rich tooltip below, excluded from the generic key/value
// fallback so they are not shown twice.
const TELEMETRY_KEYS = new Set([
	"chart",
	"event",
	"source",
	"symbol",
	"category",
	"observation",
	"bands",
	"band_labels",
	"shares",
	"calibrating",
	"calibrated",
	"samples",
	"min_samples",
	"entropy_trust",
	"confidence",
	"surprise",
	"surprise_threshold",
	"snr",
]);

const numberArray = (value: unknown): number[] | null =>
	Array.isArray(value) && value.every((item) => typeof item === "number")
		? (value as number[])
		: null;

const stringArray = (value: unknown): string[] | null =>
	Array.isArray(value) && value.every((item) => typeof item === "string")
		? (value as string[])
		: null;

const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const SignalGaugeTooltip = ({
	payload,
}: {
	payload: Record<string, unknown>;
}) => {
	const labels = stringArray(payload.band_labels);
	const shares = numberArray(payload.shares);
	const bands = numberArray(payload.bands);
	const category =
		typeof payload.category === "string" ? payload.category : null;
	const confidence = finiteNumber(payload.confidence);
	const surprise = finiteNumber(payload.surprise) ?? finiteNumber(payload.snr);
	const observation = finiteNumber(payload.observation);
	const samples = finiteNumber(payload.samples);
	const minSamples = finiteNumber(payload.min_samples);
	const entropyTrust = finiteNumber(payload.entropy_trust);
	const calibrating = payload.calibrating === true;
	const calibrated = payload.calibrated === true;

	const hasMix =
		labels !== null && shares !== null && labels.length === shares.length;
	const generic = gaugePayloadEntries(payload).filter(
		([key]) => !TELEMETRY_KEYS.has(key),
	);

	return (
		<div className="flex flex-col gap-1.5 font-mono leading-tight">
			{category !== null || calibrating ? (
				<div className="flex items-center justify-between gap-2">
					<span className="font-semibold text-foreground">
						{category ?? "—"}
					</span>
					{calibrated ? (
						<span className="rounded bg-emerald-500/15 px-1 text-[10px] text-emerald-400">
							self-calibrating
						</span>
					) : calibrating ? (
						<span className="rounded bg-amber-500/15 px-1 text-[10px] text-amber-400">
							warming up {samples ?? 0}/{minSamples ?? "?"}
						</span>
					) : null}
				</div>
			) : null}

			{confidence !== null ||
			surprise !== null ||
			observation !== null ||
			entropyTrust !== null ? (
				<div className="flex gap-3 text-[11px] text-muted-foreground">
					{confidence !== null ? (
						<span>conf {confidence.toFixed(2)}</span>
					) : null}
					{surprise !== null ? (
						<span>surprise {surprise.toFixed(2)}</span>
					) : null}
					{observation !== null ? (
						<span>obs {observation.toFixed(3)}</span>
					) : null}
					{entropyTrust !== null ? (
						<span>trust {entropyTrust.toFixed(2)}</span>
					) : null}
				</div>
			) : null}

			{hasMix && labels && shares ? (
				<div className="flex flex-col gap-0.5">
					{labels.map((bandLabel, index) => {
						const pct = shares[index] * 100;
						const active = category === bandLabel;

						return (
							<div key={bandLabel} className="flex items-center gap-1.5">
								<span
									className={`w-20 shrink-0 truncate text-[10px] ${
										active ? "text-sky-300" : "text-muted-foreground"
									}`}
								>
									{bandLabel}
								</span>
								<div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
									<div
										className={`h-full rounded-full ${
											active ? "bg-sky-400" : "bg-muted-foreground/40"
										}`}
										style={{ width: `${Math.max(0, Math.min(100, pct))}%` }}
									/>
								</div>
								<span className="w-8 shrink-0 text-right text-[10px] text-muted-foreground">
									{pct.toFixed(0)}%
								</span>
							</div>
						);
					})}
				</div>
			) : null}

			{bands !== null || samples !== null ? (
				<div className="text-[10px] text-muted-foreground">
					{bands !== null
						? `${calibrated ? "bands" : "seed"} [${bands
								.map((edge) => edge.toFixed(2))
								.join(", ")}]`
						: ""}
					{bands !== null && samples !== null ? " · " : ""}
					{samples !== null ? `n=${samples}` : ""}
				</div>
			) : null}

			{generic.length > 0 ? (
				<dl className="grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5">
					{generic.map(([key, value]) => (
						<div className="contents" key={key}>
							<dt className="text-muted-foreground">{key}</dt>
							<dd className="text-foreground tabular-nums">{value}</dd>
						</div>
					))}
				</dl>
			) : null}
		</div>
	);
};

export const SignalGauge = ({
	bridge,
	label,
}: {
	bridge: SignalGaugeBridge;
	label: string;
}) => {
	const [hovered, setHovered] = useState(false);
	const [warmupPercent, setWarmupPercent] = useState(0);
	const [tooltipPayload, setTooltipPayload] = useState<Record<
		string,
		unknown
	> | null>(null);

	const gaugeControlsRef = useRef<SignalGaugeControls | null>(null);

	const pushGaugeWire = useCallback(
		(wire: Record<string, unknown>) => {
			bridge.latest = wire;

			const controls = gaugeControlsRef.current;

			if (!controls) {
				return;
			}

			controls.updateConfidence(confidenceFromGaugePayload(wire));

			const surprise = surpriseFromGaugePayload(wire);
			const threshold = surpriseThresholdFromGaugePayload(wire) ?? 2;

			controls.updateSurprise(
				surprise,
				surpriseScaleMax(wire, surprise),
				threshold,
			);

			const percent = gaugeWarmupPercent(wire);

			setWarmupPercent(percent === null ? -1 : percent);
		},
		[bridge],
	);

	const onGaugeInit = useCallback(
		(result: TResolvedReturnType<typeof drawSignalGauge>) => {
			bridge.latest = {};
			gaugeControlsRef.current = result.controls;

			bridge.update = pushGaugeWire;
			bridge.ready = true;

			for (const pendingWire of bridge.pending) {
				pushGaugeWire(pendingWire);
			}

			bridge.pending = [];

			return () => {
				gaugeControlsRef.current = null;
				bridge.update = () => {};
				bridge.ready = false;
				bridge.pending = [];
				bridge.latest = {};
				setWarmupPercent(0);
			};
		},
		[bridge, pushGaugeWire],
	);

	useEffect(() => {
		if (!hovered) {
			setTooltipPayload(null);

			return;
		}

		setTooltipPayload({ ...bridge.latest });

		const interval = window.setInterval(() => {
			setTooltipPayload({ ...bridge.latest });
		}, 200);

		return () => window.clearInterval(interval);
	}, [hovered, bridge]);

	const showTooltip =
		tooltipPayload !== null && gaugePayloadEntries(tooltipPayload).length > 0;

	return (
		<Frame className="flex h-full min-h-0 w-full flex-col">
			<Card className="min-h-0 flex-1 overflow-hidden">
				<CardPanel className="h-full w-full p-0">
					<div
						className="relative h-full w-full"
						onPointerEnter={() => setHovered(true)}
						onPointerLeave={() => setHovered(false)}
					>
						<SciChartReact
							initChart={drawSignalGauge}
							onInit={onGaugeInit}
							style={{ height: "100%", width: "100%" }}
						/>
						{warmupPercent >= 0 ? (
							<div className="absolute inset-x-2 bottom-[15%] z-20 rounded-md bg-background/90 px-2 py-1.5">
								<Progress value={warmupPercent} className="gap-1">
									<div className="flex items-center justify-between gap-2">
										<ProgressLabel className="font-normal text-[10px] text-muted-foreground">
											Warming up
										</ProgressLabel>
										<ProgressValue className="text-[10px] text-muted-foreground" />
									</div>
									<ProgressTrack>
										<ProgressIndicator className="bg-amber-400" />
									</ProgressTrack>
								</Progress>
							</div>
						) : null}
						{showTooltip ? (
							<div
								className="pointer-events-none absolute inset-x-0 top-2 z-10 mx-auto w-max max-w-[min(100%,17rem)] rounded-md border bg-popover px-2 py-1 text-popover-foreground text-xs shadow-md/5"
								role="tooltip"
							>
								<SignalGaugeTooltip payload={tooltipPayload} />
							</div>
						) : null}
					</div>
				</CardPanel>
			</Card>
			<FrameFooter className="shrink-0 py-1">
				<div className="flex gap-1 text-muted-foreground text-xs">
					<CircleAlertIcon className="size-3 h-lh shrink-0" />
					<p>{label}</p>
				</div>
			</FrameFooter>
		</Frame>
	);
};
