import { createRef } from "react";
import {
	clearCanvas,
	drawGrid,
	drawMatrix,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import type { Measurement } from "#/types/measurement";

const hawkesCanvasRef = createRef<HTMLCanvasElement>();

/*
paintHawkes draws the current DRAW batch of hawkes measurements into
hawkesCanvasRef. Only this batch is used — nothing is retained in JS.
*/
export const paintHawkes = (value: unknown, focusSymbol: string) => {
	const canvas = hawkesCanvasRef.current;

	if (canvas === null) {
		return;
	}

	const context = resizeCanvas(canvas);

	if (context === null) {
		return;
	}

	const width = canvas.clientWidth;
	const height = canvas.clientHeight;
	const measurements = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as Measurement[];
	const hawkes = measurements.filter(
		(measurement) =>
			measurement.source === "hawkes" &&
			(focusSymbol === "" || measurement.symbol === focusSymbol),
	);
	const latestAt = hawkes.at(-1)?.at;
	const epoch =
		latestAt === undefined
			? []
			: hawkes.filter((measurement) => measurement.at === latestAt);

	const raw = (metric: string, side = ""): number | undefined => {
		for (let index = epoch.length - 1; index >= 0; index -= 1) {
			const measurement = epoch[index];

			if (measurement === undefined) {
				continue;
			}

			if (
				measurement.metric === metric &&
				(side === "" || (measurement.side ?? "") === side)
			) {
				return measurement.raw;
			}

			const key = side === "" ? metric : `${metric}:${side}`;
			const mapped = measurement.metrics?.[key];

			if (typeof mapped === "number" && Number.isFinite(mapped)) {
				return mapped;
			}
		}

		return undefined;
	};

	const values = [
		raw("baseline_intensity", "buy"),
		raw("baseline_intensity", "sell"),
		raw("conditional_intensity", "buy") ?? raw("arrival_rate", "buy"),
		raw("conditional_intensity", "sell") ?? raw("arrival_rate", "sell"),
		raw("spectral_radius"),
	].filter((entry): entry is number => typeof entry === "number");

	const fallback = epoch.at(-1)?.metrics;
	const resolved =
		values.length > 0
			? values
			: [
					fallback?.baseline,
					fallback?.intensity,
					fallback?.buyIntensity,
					fallback?.sellIntensity,
					fallback?.branching,
					fallback?.radius,
				].filter((entry): entry is number => typeof entry === "number");

	if (resolved.length === 0) {
		clearCanvas(context, width, height);
		drawGrid(context, width, height);
		context.fillStyle = TERMINAL_COLORS.muted;
		context.font = "11px JetBrains Mono, monospace";
		context.fillText("waiting for hawkes output", 18, 52);
		return;
	}

	drawMatrix(context, width, height, [resolved]);
};

/*
HawkesChart is the static canvas shell. KernelList paints it via paintHawkes.
*/
export const HawkesChart = () => (
	<canvas ref={hawkesCanvasRef} className="block size-full" />
);
