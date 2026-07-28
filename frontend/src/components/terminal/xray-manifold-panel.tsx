import { createRef } from "react";
import type { ManifoldFrame, Measurement } from "#/collections/types";
import {
	finiteMetric,
	formatMetric,
	hawkesMetricsFromBuffer,
	manifoldReading,
	signedMetric,
} from "#/components/terminal/xray-view";
import { frameRows } from "#/providers/frame-history";

const divergenceRef = createRef<HTMLSpanElement>();
const coherenceManifoldRef = createRef<HTMLSpanElement>();
const guidanceRef = createRef<HTMLSpanElement>();
const viscosityRef = createRef<HTMLSpanElement>();
const momentumRef = createRef<HTMLSpanElement>();
const fillRef = createRef<HTMLDivElement>();

const focusedFrame = <T extends { symbol: string }>(
	value: unknown,
	focusSymbol: string,
): T | null => {
	const frames = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as T[];

	return (
		frames.find(
			(entry) => focusSymbol === "" || entry.symbol === focusSymbol,
		) ??
		frames.at(-1) ??
		null
	);
};

const writeMomentum = (momentumShare: number) => {
	if (momentumRef.current !== null) {
		momentumRef.current.textContent = `${momentumShare.toFixed(2)} / 0.40`;
		momentumRef.current.style.color =
			momentumShare >= 0.4 ? "var(--up)" : "var(--f3)";
	}

	if (fillRef.current !== null) {
		fillRef.current.style.width = `${Math.min(100, momentumShare * 100)}%`;
		fillRef.current.style.background =
			momentumShare >= 0.4 ? "var(--success)" : "var(--info)";
	}
};

/*
paintXrayManifold paints nested manifold.reading scalars from the current DRAW
manifold batch into the manifold side panel.
*/
export const paintXrayManifold = (value: unknown, focusSymbol: string) => {
	const frame = focusedFrame<ManifoldFrame>(value, focusSymbol);
	const reading = manifoldReading(frame);
	const momentumShare = finiteMetric(reading?.coherenceMag2) ?? 0;

	if (divergenceRef.current !== null) {
		divergenceRef.current.textContent = signedMetric(
			finiteMetric(reading?.divergence),
		);
	}

	if (coherenceManifoldRef.current !== null) {
		coherenceManifoldRef.current.textContent = formatMetric(
			finiteMetric(reading?.coherenceMag2),
		);
	}

	if (guidanceRef.current !== null) {
		guidanceRef.current.textContent = formatMetric(
			finiteMetric(reading?.guidanceSpeed),
		);
	}

	if (viscosityRef.current !== null) {
		viscosityRef.current.textContent = formatMetric(
			finiteMetric(reading?.viscosityProxy),
		);
	}

	writeMomentum(momentumShare);
};

/*
paintXrayManifoldMeasurements updates momentum eigenmode from the latest
retained Hawkes measurements when manifold coherence is unavailable.
*/
export const paintXrayManifoldMeasurements = (
	value: unknown,
	focusSymbol: string,
) => {
	if (momentumRef.current === null) {
		return;
	}

	const measurements = frameRows<Measurement>(value);
	const hawkes = hawkesMetricsFromBuffer(
		measurements.filter(
			(measurement) =>
				measurement.source === "hawkes" &&
				(focusSymbol === "" || measurement.symbol === focusSymbol),
		),
	);
	const share = hawkes.radius ?? hawkes.branching;

	if (share === null) {
		return;
	}

	const coherenceText = coherenceManifoldRef.current?.textContent ?? "";

	if (coherenceText !== "" && coherenceText !== "—") {
		return;
	}

	writeMomentum(share);
};

/*
XrayManifoldPanel is the static manifold reading shell. DRAW paints via
paintXrayManifold and paintXrayManifoldMeasurements.
*/
export const XrayManifoldPanel = () => (
	<div className="flex flex-col gap-2 border-(--line) border-t px-3.5 py-3">
		<div>
			<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
				Manifold reading
			</div>
			<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
				|ψ|² · guidance current · particles
			</div>
		</div>
		<div className="grid grid-cols-2 gap-x-4 gap-y-2 font-mono text-[11px]">
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">∇·u</span>
				<span ref={divergenceRef} className="text-right text-(--f1)" />
			</div>
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">|ψ|²</span>
				<span ref={coherenceManifoldRef} className="text-right text-(--f1)" />
			</div>
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">guide v</span>
				<span ref={guidanceRef} className="text-right text-(--f1)" />
			</div>
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">viscosity</span>
				<span ref={viscosityRef} className="text-right text-(--f1)" />
			</div>
		</div>
		<div className="mt-0.5">
			<div className="mb-1 flex justify-between text-[10px]">
				<span className="text-(--f3)">momentum eigenmode</span>
				<span ref={momentumRef} className="font-mono" />
			</div>
			<div className="relative">
				<div className="h-1.5 overflow-hidden rounded-[3px] bg-(--line)">
					<div ref={fillRef} className="h-full" style={{ width: "0%" }} />
				</div>
				<div className="relative h-0">
					<div className="absolute -top-2.25 left-[40%] h-3 w-0.5 bg-(--acc)" />
				</div>
			</div>
			<div className="mt-1.5 font-mono text-[8.5px] text-(--f4)">
				drive playbook gate · mode share ≥ 0.40
			</div>
		</div>
	</div>
);
