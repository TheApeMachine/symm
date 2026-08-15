import { useCallback, useEffect, useRef } from "react";
import {
	clearCanvas,
	drawGrid,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import { requirePositiveLength } from "#/lib/domain";

export {
	paintTerminalFluidChart,
	TerminalFluidChart,
} from "#/components/charts/fluid";
export {
	paintTerminalManifoldChart,
	TerminalManifoldChart,
} from "#/components/charts/manifold";
export { TerminalPhaseDialChart } from "#/components/charts/phase-dial";
export { TerminalPredictionChart } from "#/components/charts/prediction";
export {
	paintTerminalResonanceChart,
	TerminalResonanceChart,
} from "#/components/charts/resonance";
export { TerminalSignalHeatmap } from "#/components/charts/signal-heatmap";
export {
	fluidGridDimensions,
	phaseColumnsFromScan,
	phaseLeadersFromScan,
	terminalPhaseScanFromFrame,
	terminalPhaseStatusFromFrame,
	terminalResonanceLayerMatrixFromFrame,
	terminalWaveModesFromFrame,
} from "#/components/terminal/charts-frame";

type Draw = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
) => void;

const drawWaiting = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	message: string,
) => {
	clearCanvas(context, width, height);
	drawGrid(context, width, height);
	context.fillStyle = TERMINAL_COLORS.muted;
	context.font = "11px JetBrains Mono, monospace";
	context.fillText(message, 18, 52);
};

const StaticCanvas = ({ draw }: { draw: Draw }) => {
	const canvasRef = useRef<HTMLCanvasElement | null>(null);

	useEffect(() => {
		const canvas = canvasRef.current;

		if (canvas === null) {
			return;
		}

		const render = () => {
			const context = resizeCanvas(canvas);

			if (context === null) {
				return;
			}

			draw(context, canvas.clientWidth, canvas.clientHeight);
		};

		render();
		const observer = new ResizeObserver(render);
		observer.observe(canvas);

		return () => observer.disconnect();
	}, [draw]);

	return <canvas ref={canvasRef} className="block size-full" />;
};

/*
TerminalCognitiveChart is a static waiting shell until cognitive paint lands.
*/
export const TerminalCognitiveChart = () => {
	const draw = useCallback<Draw>((context, width, height) => {
		drawWaiting(context, width, height, "waiting for cognitive frames");
	}, []);

	return <StaticCanvas draw={draw} />;
};

/*
TerminalPositionChart paints open-lot PnL bars from props (not DRAW).
*/
export const TerminalPositionChart = ({
	positions,
}: {
	positions: Array<{
		key: string;
		symbol: string;
		pnlPercentText: string;
		profitable: boolean;
	}>;
}) => {
	const draw = useCallback<Draw>(
		(context, width, height) => {
			clearCanvas(context, width, height);
			drawGrid(context, width, height);

			if (positions.length === 0) {
				return;
			}

			requirePositiveLength(positions.length, "position chart rows");
			const rowHeight = Math.max(22, (height - 24) / positions.length);

			positions.forEach((position, index) => {
				const y = 18 + index * rowHeight;
				const value = Number.parseFloat(position.pnlPercentText) || 0;
				const bar = Math.min(width * 0.42, Math.abs(value) * 18);
				const origin = width * 0.5;

				context.fillStyle = TERMINAL_COLORS.muted;
				context.font = "10px JetBrains Mono, monospace";
				context.fillText(position.symbol, 12, y + 4);
				context.fillStyle = position.profitable
					? TERMINAL_COLORS.green
					: TERMINAL_COLORS.red;
				context.fillRect(value >= 0 ? origin : origin - bar, y - 6, bar, 10);
			});
		},
		[positions],
	);

	return <StaticCanvas draw={draw} />;
};
