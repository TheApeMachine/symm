import { useEffect, useRef } from "react";

type KuramotoPhase = {
	phase: number;
	heat: number;
};

export type KuramotoRingProps = {
	oscillators: KuramotoPhase[];
	kuramotoR: number;
	kuramotoPsi: number;
};

const TAU = 2 * Math.PI;
const RING_PADDING = 16;

const heatColor = (heat: number): string => {
	const clamped = Math.max(0, Math.min(1, heat));
	const red = Math.round(60 + 195 * clamped);
	const green = Math.round(40 + 120 * (1 - clamped * 0.6));
	const blue = Math.round(180 * (1 - clamped));

	return `rgb(${red},${green},${blue})`;
};

const paint = (canvas: HTMLCanvasElement, props: KuramotoRingProps) => {
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

	const centerX = rect.width / 2;
	const centerY = rect.height / 2;
	const radius = Math.min(centerX, centerY) - RING_PADDING;

	// Unit circle ring
	context.strokeStyle = "rgba(255,255,255,0.08)";
	context.lineWidth = 1;
	context.beginPath();
	context.arc(centerX, centerY, radius, 0, TAU);
	context.stroke();

	// Oscillator phasors as dots on the ring
	for (const oscillator of props.oscillators) {
		const plotX = centerX + Math.cos(oscillator.phase) * radius;
		const plotY = centerY - Math.sin(oscillator.phase) * radius;

		context.fillStyle = heatColor(oscillator.heat);
		context.beginPath();
		context.arc(plotX, plotY, 3, 0, TAU);
		context.fill();
	}

	// Resultant vector R∠Ψ
	const arrowLength = props.kuramotoR * radius;
	const arrowEndX = centerX + Math.cos(props.kuramotoPsi) * arrowLength;
	const arrowEndY = centerY - Math.sin(props.kuramotoPsi) * arrowLength;

	context.strokeStyle = "rgba(255,180,60,0.9)";
	context.lineWidth = 2;
	context.beginPath();
	context.moveTo(centerX, centerY);
	context.lineTo(arrowEndX, arrowEndY);
	context.stroke();

	// Arrow head
	const headLength = 6;
	const arrowAngle = Math.atan2(-(arrowEndY - centerY), arrowEndX - centerX);
	context.fillStyle = "rgba(255,180,60,0.9)";
	context.beginPath();
	context.moveTo(arrowEndX, arrowEndY);
	context.lineTo(
		arrowEndX - headLength * Math.cos(arrowAngle - 0.4),
		arrowEndY + headLength * Math.sin(arrowAngle - 0.4),
	);
	context.lineTo(
		arrowEndX - headLength * Math.cos(arrowAngle + 0.4),
		arrowEndY + headLength * Math.sin(arrowAngle + 0.4),
	);
	context.closePath();
	context.fill();

	// R readout
	context.fillStyle = "rgba(255,255,255,0.6)";
	context.font = "10px monospace";
	context.textAlign = "center";
	context.fillText(
		`R = ${props.kuramotoR.toFixed(3)}`,
		centerX,
		rect.height - 4,
	);
};

export const KuramotoRing = (props: KuramotoRingProps) => {
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
