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

let smoothedMaxDiv = 0.01;
let smoothedMaxPres = 0.01;

const paint = (
	canvas: HTMLCanvasElement,
	props: PhasePortraitProps,
) => {
	const context = canvas.getContext("2d");

	if (context === null) {
		return;
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

	const plotLeft = PADDING;
	const plotRight = rect.width - PADDING;
	const plotTop = PADDING - 8;
	const plotBottom = rect.height - PADDING + 4;
	const plotWidth = plotRight - plotLeft;
	const plotHeight = plotBottom - plotTop;
	const centerX = (plotLeft + plotRight) / 2;

	const points = props.history.slice(-TAIL_LENGTH);

	if (points.length === 0) {
		return;
	}

	// Calculate peak absolute divergence and peak pressure
	let peakDiv = Math.abs(props.current.divergence);
	let peakPres = Math.abs(props.current.pressureGradNorm);

	for (const point of points) {
		peakDiv = Math.max(peakDiv, Math.abs(point.divergence));
		peakPres = Math.max(peakPres, Math.abs(point.pressureGradNorm));
	}

	// Smooth the bounds with exponential moving average so the axes don't jitter
	smoothedMaxDiv = smoothedMaxDiv * 0.9 + Math.max(peakDiv * 1.25, 0.0001) * 0.1;
	smoothedMaxPres = smoothedMaxPres * 0.9 + Math.max(peakPres * 1.25, 0.0001) * 0.1;

	const toPlotX = (divergence: number) => {
		const norm = divergence / smoothedMaxDiv;
		return centerX + (norm * plotWidth) / 2;
	};

	const toPlotY = (pressure: number) => {
		const norm = Math.max(0, pressure) / smoothedMaxPres;
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

	// Smooth fading trail
	if (points.length > 1) {
		for (let index = 1; index < points.length; index += 1) {
			const opacity = Math.pow(index / points.length, 2) * 0.6;
			const radius = 1 + 2 * (index / points.length);
			const x = toPlotX(points[index].divergence);
			const y = toPlotY(points[index].pressureGradNorm);

			context.fillStyle = `rgba(80,170,240,${opacity})`;
			context.beginPath();
			context.arc(x, y, radius, 0, 2 * Math.PI);
			context.fill();

			// Connecting link to previous
			const prevX = toPlotX(points[index - 1].divergence);
			const prevY = toPlotY(points[index - 1].pressureGradNorm);
			context.strokeStyle = `rgba(80,170,240,${opacity * 0.5})`;
			context.lineWidth = 1;
			context.beginPath();
			context.moveTo(prevX, prevY);
			context.lineTo(x, y);
			context.stroke();
		}
	}

	// Current active state
	const currentX = toPlotX(props.current.divergence);
	const currentY = toPlotY(props.current.pressureGradNorm);

	// Glow aura
	const gradient = context.createRadialGradient(
		currentX, currentY, 0,
		currentX, currentY, 10,
	);
	gradient.addColorStop(0, "rgba(255,180,60,0.8)");
	gradient.addColorStop(1, "rgba(255,180,60,0.0)");
	context.fillStyle = gradient;
	context.beginPath();
	context.arc(currentX, currentY, 10, 0, 2 * Math.PI);
	context.fill();

	// Solid head
	context.fillStyle = "rgba(255,210,100,1.0)";
	context.beginPath();
	context.arc(currentX, currentY, 3.5, 0, 2 * Math.PI);
	context.fill();

	// Live value overlay
	context.fillStyle = "rgba(255,200,80,0.8)";
	context.font = "8px monospace";
	context.textAlign = "left";
	context.fillText(
		`∇·u: ${props.current.divergence >= 0 ? "+" : ""}${props.current.divergence.toFixed(4)}`,
		plotLeft,
		plotTop + 10,
	);
	context.fillText(
		`‖∇P‖: ${props.current.pressureGradNorm.toFixed(4)}`,
		plotLeft,
		plotTop + 20,
	);
};

export const PhasePortrait = (props: PhasePortraitProps) => {
	const canvasRef = useRef<HTMLCanvasElement>(null);

	useEffect(() => {
		const canvas = canvasRef.current;

		if (canvas === null) {
			return;
		}

		paint(canvas, props);
	}, [props]);

	return (
		<canvas
			ref={canvasRef}
			className="size-full"
			style={{ display: "block" }}
		/>
	);
};
