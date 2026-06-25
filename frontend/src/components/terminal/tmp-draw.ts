import { TERMINAL_COLORS } from "#/components/terminal/canvas";
import type {
	FluidSim,
	HawkesSim,
	ManifoldPoint,
	PredBuffer,
	WhaleCarrier,
} from "#/components/terminal/tmp-sim";
import { advanceFluidSim, clamp } from "#/components/terminal/tmp-sim";

const accentHex = (): string => TERMINAL_COLORS.amber;

const lerpRgb = (
	left: [number, number, number],
	right: [number, number, number],
	amount: number,
): [number, number, number] => [
	Math.round(left[0] + (right[0] - left[0]) * amount),
	Math.round(left[1] + (right[1] - left[1]) * amount),
	Math.round(left[2] + (right[2] - left[2]) * amount),
];

const hexToRgb = (hex: string): [number, number, number] => [
	parseInt(hex.slice(1, 3), 16),
	parseInt(hex.slice(3, 5), 16),
	parseInt(hex.slice(5, 7), 16),
];

export const terminalColormap = (value: number): [number, number, number] => {
	const accent = hexToRgb(accentHex());
	const stops: Array<[number, [number, number, number]]> = [
		[0, [14, 12, 10]],
		[0.4, [26, 34, 50]],
		[0.6, [42, 106, 129]],
		[0.8, accent],
		[1, lerpRgb(accent, [255, 248, 224], 0.6)],
	];
	const normalized = clamp(value, 0, 1);

	for (let index = 0; index < stops.length - 1; index += 1) {
		const current = stops[index];
		const next = stops[index + 1];

		if (current === undefined || next === undefined) {
			continue;
		}

		if (normalized <= next[0]) {
			const span = next[0] - current[0] || 1;
			const local = (normalized - current[0]) / span;

			return lerpRgb(current[1], next[1], local);
		}
	}

	return stops.at(-1)?.[1] ?? accent;
};

export const drawTmpFluid = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	sim: FluidSim,
): number => {
	const columns = 64;
	const rows = 38;
	const cellWidth = width / columns;
	const cellHeight = height / rows;
	const time = performance.now() / 1000;
	let peak = 0;

	advanceFluidSim(sim);

	for (let row = 0; row < rows; row += 1) {
		for (let column = 0; column < columns; column += 1) {
			const x = column / columns;
			const y = row / rows;
			let value = 0;

			for (const blob of sim.blobs) {
				const deltaX = x - blob.x;
				const deltaY = y - blob.y;
				value +=
					blob.a *
					Math.exp(
						-(deltaX * deltaX + deltaY * deltaY) / (2 * blob.r * blob.r),
					);
			}

			value +=
				0.08 * Math.sin(x * 14 + time * 0.7) * Math.cos(y * 9 - time * 0.5);
			value = clamp(value / 1.6, 0, 1);

			if (value > peak) {
				peak = value;
			}

			const color = terminalColormap(value);
			context.fillStyle = `rgb(${color[0]},${color[1]},${color[2]})`;
			context.fillRect(
				column * cellWidth,
				row * cellHeight,
				cellWidth + 1,
				cellHeight + 1,
			);
		}
	}

	drawWhales(context, width, height, sim.whales);
	sim.peak = peak;

	return peak;
};

const drawWhales = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	whales: WhaleCarrier[],
): void => {
	const accent = accentHex();

	for (const whale of whales) {
		const px = whale.x * width;
		const py = whale.y * height;
		const gradient = context.createRadialGradient(px, py, 0, px, py, 22);
		gradient.addColorStop(0, accent);
		gradient.addColorStop(1, "rgba(0,0,0,0)");
		context.fillStyle = gradient;
		context.beginPath();
		context.arc(px, py, 22, 0, Math.PI * 2);
		context.fill();
		context.fillStyle = "#fff";
		context.beginPath();
		context.arc(px, py, 2.4, 0, Math.PI * 2);
		context.fill();
	}
};

export const drawTmpPrediction = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	buffer: PredBuffer,
): void => {
	context.clearRect(0, 0, width, height);

	if (buffer.actual.length < 2) {
		return;
	}

	const count = buffer.actual.length;
	const pad = 14;
	const xAt = (index: number) =>
		pad + (index / (count - 1)) * (width - pad * 2);
	const yAt = (value: number) => height - 26 - value * (height - 46);

	context.strokeStyle = "#241f19";
	context.lineWidth = 1;

	for (let grid = 0; grid <= 4; grid += 1) {
		const y = 18 + grid * ((height - 44) / 4);
		context.beginPath();
		context.moveTo(pad, y);
		context.lineTo(width - pad, y);
		context.stroke();
	}

	context.beginPath();

	for (let index = 0; index < count; index += 1) {
		context.lineTo(xAt(index), yAt(buffer.actual[index] ?? 0));
	}

	for (let index = count - 1; index >= 0; index -= 1) {
		context.lineTo(xAt(index), yAt(buffer.pred[index] ?? 0));
	}

	context.closePath();
	context.fillStyle = `${accentHex()}2e`;
	context.fill();

	context.strokeStyle = TERMINAL_COLORS.cyan;
	context.lineWidth = 1.5;
	context.setLineDash([4, 3]);
	context.beginPath();

	for (let index = 0; index < count; index += 1) {
		const x = xAt(index);
		const y = yAt(buffer.pred[index] ?? 0);

		if (index === 0) {
			context.moveTo(x, y);
			continue;
		}

		context.lineTo(x, y);
	}

	context.stroke();
	context.setLineDash([]);

	context.strokeStyle = TERMINAL_COLORS.foreground;
	context.lineWidth = 1.8;
	context.beginPath();

	for (let index = 0; index < count; index += 1) {
		const x = xAt(index);
		const y = yAt(buffer.actual[index] ?? 0);

		if (index === 0) {
			context.moveTo(x, y);
			continue;
		}

		context.lineTo(x, y);
	}

	context.stroke();
	context.fillStyle = accentHex();
	context.beginPath();
	context.arc(
		xAt(count - 1),
		yAt(buffer.actual[count - 1] ?? 0),
		2.6,
		0,
		Math.PI * 2,
	);
	context.fill();
};

export const drawTmpHawkes = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	hawkes: HawkesSim,
): void => {
	context.clearRect(0, 0, width, height);

	const buffer = hawkes.buf;

	if (buffer.length < 2) {
		return;
	}

	const maxLambda = Math.max(1.2, ...buffer) * 1.1;
	const pad = 14;
	const base = height - 26;
	const xAt = (index: number) =>
		pad + (index / (buffer.length - 1)) * (width - pad * 2);
	const yAt = (value: number) => base - (value / maxLambda) * (base - 30);

	context.strokeStyle = TERMINAL_COLORS.lineStrong;
	context.setLineDash([3, 3]);
	context.lineWidth = 1;
	context.beginPath();
	context.moveTo(pad, yAt(hawkes.mu));
	context.lineTo(width - pad, yAt(hawkes.mu));
	context.stroke();
	context.setLineDash([]);

	const accent = accentHex();
	context.beginPath();
	context.moveTo(xAt(0), base);

	for (let index = 0; index < buffer.length; index += 1) {
		context.lineTo(xAt(index), yAt(buffer[index] ?? 0));
	}

	context.lineTo(xAt(buffer.length - 1), base);
	context.closePath();
	context.fillStyle = `${accent}24`;
	context.fill();
	context.strokeStyle = accent;
	context.lineWidth = 1.6;
	context.beginPath();

	for (let index = 0; index < buffer.length; index += 1) {
		const x = xAt(index);
		const y = yAt(buffer[index] ?? 0);

		if (index === 0) {
			context.moveTo(x, y);
			continue;
		}

		context.lineTo(x, y);
	}

	context.stroke();

	const now = performance.now();
	context.strokeStyle = TERMINAL_COLORS.cyan;
	context.lineWidth = 1;

	for (const event of hawkes.events) {
		const age = now - event;

		if (age > 6000) {
			continue;
		}

		const x = width - pad - (age / 6000) * (width - pad * 2);
		context.globalAlpha = Math.max(0, 1 - age / 6000);
		context.beginPath();
		context.moveTo(x, base);
		context.lineTo(x, base + 8);
		context.stroke();
	}

	context.globalAlpha = 1;
};

export const drawTmpManifold = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	points: ManifoldPoint[],
	focusSymbol: string,
): void => {
	context.clearRect(0, 0, width, height);

	const pad = 26;
	const xAt = (value: number) =>
		pad + ((value + 1.15) / 2.3) * (width - pad * 2);
	const yAt = (value: number) =>
		pad + ((value + 1.15) / 2.3) * (height - pad * 2);
	const palette = [
		TERMINAL_COLORS.cyan,
		accentHex(),
		TERMINAL_COLORS.green,
		TERMINAL_COLORS.red,
	];

	context.strokeStyle = "#241f19";
	context.lineWidth = 1;

	for (let grid = 0; grid <= 4; grid += 1) {
		const gridX = pad + grid * ((width - pad * 2) / 4);
		const gridY = pad + grid * ((height - pad * 2) / 4);
		context.beginPath();
		context.moveTo(gridX, pad);
		context.lineTo(gridX, height - pad);
		context.moveTo(pad, gridY);
		context.lineTo(width - pad, gridY);
		context.stroke();
	}

	for (const point of points) {
		const x = xAt(point.lx);
		const y = yAt(point.ly);
		const focal =
			point.symbol === focusSymbol.split("/")[0] ||
			point.symbol === focusSymbol;
		const radius = focal ? 6 : 2.4 + point.vol * 2.2;
		context.fillStyle = palette[point.cluster] ?? palette[0];
		context.globalAlpha = focal ? 1 : 0.6;
		context.beginPath();
		context.arc(x, y, radius, 0, Math.PI * 2);
		context.fill();

		if (focal) {
			context.globalAlpha = 1;
			context.strokeStyle = accentHex();
			context.lineWidth = 1.4;
			context.beginPath();
			context.arc(
				x,
				y,
				11 + 2 * Math.sin(performance.now() / 300),
				0,
				Math.PI * 2,
			);
			context.stroke();
			context.fillStyle = TERMINAL_COLORS.foreground;
			context.font = "10px JetBrains Mono, monospace";
			context.fillText(point.symbol, x + 13, y + 3);
		}
	}

	context.globalAlpha = 1;
};
