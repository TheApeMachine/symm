import { useEffect, useRef } from "react";

export type PhasePortraitPoint = {
	divergence: number;
	pressureGradNorm: number;
};

export type PhasePortraitProps = {
	history: PhasePortraitPoint[];
	current: PhasePortraitPoint;
};

const PADDING = 28;
const TAIL_LENGTH = 45;
const INITIAL_SCALE = 0.01;

export const finitePortraitPoint = (
	divergence: unknown,
	pressureGradNorm: unknown,
): PhasePortraitPoint | null => {
	if (
		typeof divergence !== "number" ||
		typeof pressureGradNorm !== "number" ||
		!Number.isFinite(divergence) ||
		!Number.isFinite(pressureGradNorm)
	) {
		return null;
	}

	return { divergence, pressureGradNorm };
};

type AxisScale = {
	divergence: number;
	pressure: number;
};

const paint = (
	canvas: HTMLCanvasElement,
	props: PhasePortraitProps,
	scale: AxisScale,
): AxisScale => {
	const context = canvas.getContext("2d");

	if (context === null) {
		return scale;
	}

	const devicePixelRatio = window.devicePixelRatio ?? 1;
	const rect = canvas.getBoundingClientRect();
	const canvasWidth = Math.round(rect.width * devicePixelRatio);
	const canvasHeight = Math.round(rect.height * devicePixelRatio);

	if (canvas.width !== canvasWidth || canvas.height !== canvasHeight) {
		canvas.width = canvasWidth;
		canvas.height = canvasHeight;
	}

	context.setTransform(devicePixelRatio, 0, 0, devicePixelRatio, 0, 0);
	context.clearRect(0, 0, rect.width, rect.height);

	if (rect.width <= 0 || rect.height <= 0) {
		return scale;
	}

	const plotLeft = PADDING;
	const plotRight = rect.width - PADDING;
	const plotTop = PADDING - 8;
	const plotBottom = rect.height - PADDING + 4;
	const plotWidth = plotRight - plotLeft;
	const plotHeight = plotBottom - plotTop;
	const centerX = (plotLeft + plotRight) / 2;
	const source = props.history.length > 0 ? props.history : [props.current];
	const points = source
		.flatMap((point) => {
			const finite = finitePortraitPoint(
				point.divergence,
				point.pressureGradNorm,
			);
			return finite === null ? [] : [finite];
		})
		.slice(-TAIL_LENGTH);

	if (points.length === 0) {
		return scale;
	}

	let peakDiv = 0;
	let peakPres = 0;

	for (const point of points) {
		peakDiv = Math.max(peakDiv, Math.abs(point.divergence));
		peakPres = Math.max(peakPres, Math.abs(point.pressureGradNorm));
	}

	const nextScale = {
		divergence: scale.divergence * 0.9 + Math.max(peakDiv * 1.25, 0.0001) * 0.1,
		pressure: scale.pressure * 0.9 + Math.max(peakPres * 1.25, 0.0001) * 0.1,
	};

	const toPlotX = (divergence: number) => {
		const norm = divergence / nextScale.divergence;
		return centerX + (norm * plotWidth) / 2;
	};

	const toPlotY = (pressure: number) => {
		const norm = Math.max(0, pressure) / nextScale.pressure;
		return plotBottom - Math.min(norm, 1) * plotHeight;
	};

	// Center zero-divergence vertical axis
	context.strokeStyle = "rgba(255,255,255,0.15)";
	context.lineWidth = 1;
	context.setLineDash([2, 3]);
	context.beginPath();
	context.moveTo(centerX, plotTop);
	context.lineTo(centerX, plotBottom);
	context.stroke();
	context.setLineDash([]);

	// Bottom baseline
	context.strokeStyle = "rgba(255,255,255,0.12)";
	context.beginPath();
	context.moveTo(plotLeft, plotBottom);
	context.lineTo(plotRight, plotBottom);
	context.stroke();

	// Axis labels & zone markers
	context.fillStyle = "rgba(255,255,255,0.4)";
	context.font = "8px monospace";
	context.textAlign = "left";
	context.fillText("compression (-)", plotLeft, rect.height - 4);
	context.textAlign = "right";
	context.fillText("expansion (+)", plotRight, rect.height - 4);
	context.textAlign = "center";
	context.fillText("0", centerX, rect.height - 4);

	if (points.length > 1) {
		for (let index = 1; index < points.length; index += 1) {
			const point = points[index];
			const previous = points[index - 1];

			if (point === undefined || previous === undefined) {
				continue;
			}

			const opacity = (index / points.length) ** 2 * 0.6;
			const radius = 1 + 2 * (index / points.length);
			const x = toPlotX(point.divergence);
			const y = toPlotY(point.pressureGradNorm);
			const prevX = toPlotX(previous.divergence);
			const prevY = toPlotY(previous.pressureGradNorm);

			if (
				!Number.isFinite(x) ||
				!Number.isFinite(y) ||
				!Number.isFinite(prevX) ||
				!Number.isFinite(prevY)
			) {
				continue;
			}

			context.fillStyle = `rgba(80,170,240,${opacity})`;
			context.beginPath();
			context.arc(x, y, radius, 0, 2 * Math.PI);
			context.fill();
			context.strokeStyle = `rgba(80,170,240,${opacity * 0.5})`;
			context.lineWidth = 1;
			context.beginPath();
			context.moveTo(prevX, prevY);
			context.lineTo(x, y);
			context.stroke();
		}
	}

	const current = points[points.length - 1];

	if (current === undefined) {
		return nextScale;
	}

	const currentX = toPlotX(current.divergence);
	const currentY = toPlotY(current.pressureGradNorm);

	if (!Number.isFinite(currentX) || !Number.isFinite(currentY)) {
		return nextScale;
	}

	const gradient = context.createRadialGradient(
		currentX,
		currentY,
		0,
		currentX,
		currentY,
		10,
	);
	gradient.addColorStop(0, "rgba(255,180,60,0.8)");
	gradient.addColorStop(1, "rgba(255,180,60,0.0)");
	context.fillStyle = gradient;
	context.beginPath();
	context.arc(currentX, currentY, 10, 0, 2 * Math.PI);
	context.fill();
	context.fillStyle = "rgba(255,210,100,1.0)";
	context.beginPath();
	context.arc(currentX, currentY, 3.5, 0, 2 * Math.PI);
	context.fill();

	// Live value overlay
	context.fillStyle = "rgba(255,200,80,0.8)";
	context.font = "8px monospace";
	context.textAlign = "left";
	context.fillText(
		`∇·u: ${current.divergence >= 0 ? "+" : ""}${current.divergence.toFixed(4)}`,
		plotLeft,
		plotTop + 10,
	);
	context.fillText(
		`‖∇P‖: ${current.pressureGradNorm.toFixed(4)}`,
		plotLeft,
		plotTop + 20,
	);

	return nextScale;
};

export const PhasePortrait = (props: PhasePortraitProps) => {
	const canvasRef = useRef<HTMLCanvasElement>(null);
	const scaleRef = useRef<AxisScale>({
		divergence: INITIAL_SCALE,
		pressure: INITIAL_SCALE,
	});

	useEffect(() => {
		const canvas = canvasRef.current;

		if (canvas === null) {
			return;
		}

		scaleRef.current = paint(canvas, props, scaleRef.current);
	}, [props]);

	return (
		<canvas
			ref={canvasRef}
			className="size-full"
			style={{ display: "block" }}
		/>
	);
};
