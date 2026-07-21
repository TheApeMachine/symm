/*
TerminalFluidParticle is one focused oscillator published by the manifold.
Its cell coordinates remain in the backend's complete X–Y–Z lattice.
*/
export type TerminalFluidParticle = {
	source: string;
	role: string;
	cellX: number;
	cellY: number;
	cellZ: number;
	phase: number;
	omega: number;
	amplitude: number;
	heat: number;
	velX: number;
	velY: number;
	velZ: number;
	speed: number;
};

/*
FluidParticleCell is the population-complete X–Z display aggregation of focused
particles. Count preserves population and amplitude preserves their coherent
resultant after the field projection has collapsed Y.
*/
export type FluidParticleCell = {
	column: number;
	row: number;
	count: number;
	amplitude: number;
};

/*
FluidParticleAccumulator retains complex components until an occupied cell is
complete, preventing premature magnitude summation from erasing interference.
*/
type FluidParticleAccumulator = FluidParticleCell & {
	real: number;
	imaginary: number;
};

/*
aggregateFluidParticles projects every focused oscillator into its X–Z cell.
Counts preserve the complete population while complex summation retains the
phase cancellation that a simple amplitude ranking would discard.
*/
export const aggregateFluidParticles = (
	particles: TerminalFluidParticle[],
	columns: number,
	rows: number,
): FluidParticleCell[] => {
	const occupied = new Map<number, FluidParticleAccumulator>();

	for (const particle of particles) {
		const column = Math.floor(particle.cellX);
		const row = Math.floor(particle.cellZ);

		if (
			column < 0 ||
			column >= columns ||
			row < 0 ||
			row >= rows ||
			!Number.isFinite(particle.phase) ||
			!Number.isFinite(particle.amplitude) ||
			particle.amplitude < 0
		) {
			continue;
		}

		const key = row * columns + column;
		const cell = occupied.get(key) ?? {
			column,
			row,
			count: 0,
			amplitude: 0,
			real: 0,
			imaginary: 0,
		};
		cell.count += 1;
		cell.real += particle.amplitude * Math.cos(particle.phase);
		cell.imaginary += particle.amplitude * Math.sin(particle.phase);
		occupied.set(key, cell);
	}

	return Array.from(occupied.values(), (cell) => ({
		column: cell.column,
		row: cell.row,
		count: cell.count,
		amplitude: Math.hypot(cell.real, cell.imaginary),
	}));
};

/*
drawFluidParticles paints one marker per occupied X–Z cell. Marker area encodes
focused particle count and glow encodes the cell's phase-coherent amplitude, so
all focused observations remain represented without an arbitrary marker cap.
*/
export const drawFluidParticles = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	particles: TerminalFluidParticle[],
	columns: number,
	rows: number,
) => {
	const cells = aggregateFluidParticles(particles, columns, rows);

	if (cells.length === 0 || columns <= 0 || rows <= 0) {
		return;
	}

	const cellWidth = width / columns;
	const cellHeight = height / rows;
	const cellDiagonal = Math.hypot(cellWidth, cellHeight);
	const maximumCount = Math.max(...cells.map((cell) => cell.count));
	const maximumAmplitude = Math.max(...cells.map((cell) => cell.amplitude));

	for (const cell of cells) {
		const countUnit = Math.sqrt(cell.count / maximumCount);
		const coreRadius = Math.sqrt(cell.count);
		const amplitudeUnit =
			maximumAmplitude > 0 ? cell.amplitude / maximumAmplitude : 0;
		const x = (cell.column + 0.5) * cellWidth;
		const y = (cell.row + 0.5) * cellHeight;
		const glowRadius = cellDiagonal * countUnit;

		if (amplitudeUnit > 0) {
			const glow = context.createRadialGradient(x, y, 0, x, y, glowRadius);
			glow.addColorStop(0, `rgba(232, 163, 61, ${amplitudeUnit})`);
			glow.addColorStop(1, "rgba(0, 0, 0, 0)");
			context.fillStyle = glow;
			context.beginPath();
			context.arc(x, y, glowRadius, 0, Math.PI * 2);
			context.fill();
		}

		context.fillStyle = "rgb(255, 255, 255)";
		context.beginPath();
		context.arc(x, y, coreRadius, 0, Math.PI * 2);
		context.fill();
	}
};
