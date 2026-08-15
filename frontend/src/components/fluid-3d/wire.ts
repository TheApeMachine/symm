export type FluidGrid = {
	x: number;
	y: number;
	z: number;
	spacing: number;
};

export type FluidFields = {
	Grid: FluidGrid;
	Density: number[];
	Momentum: number[];
	InternalEnergy: number[];
	WaveReal: number[];
	WaveImaginary: number[];
};

export type FluidVector = {
	X: number;
	Y: number;
	Z: number;
};

export type FluidParticle = {
	Position: FluidVector;
	Velocity: FluidVector;
	Mass: number;
	Heat: number;
	Energy: number;
	Phase: number;
	Omega: number;
};

type FluidEnvelope = {
	fields?: unknown;
	particles?: unknown;
};

export const decodePhase = (value: unknown): Record<string, unknown> =>
	objectValue(value, "fluid phase");

const objectValue = (value: unknown, name: string): Record<string, unknown> => {
	if (value === null || typeof value !== "object" || Array.isArray(value)) {
		throw new Error(`${name} must be an object`);
	}

	return value as Record<string, unknown>;
};

const finiteNumber = (value: unknown, name: string) => {
	if (typeof value !== "number" || !Number.isFinite(value)) {
		throw new Error(`${name} must be a finite number`);
	}

	return value;
};

const positiveInteger = (value: unknown, name: string) => {
	const number = finiteNumber(value, name);

	if (!Number.isInteger(number) || number <= 0) {
		throw new Error(`${name} must be a positive integer`);
	}

	return number;
};

const numberArray = (value: unknown, length: number, name: string) => {
	if (!Array.isArray(value) || value.length !== length) {
		throw new Error(`${name} must contain ${length} values`);
	}

	return value as number[];
};

export const decodeFields = (value: unknown): FluidFields => {
	const envelope = objectValue(value, "fluid envelope") as FluidEnvelope;
	const fields = objectValue(envelope.fields, "fields");
	const gridValue = objectValue(fields.Grid, "fields.Grid");
	const Grid = {
		x: positiveInteger(gridValue.x, "fields.Grid.x"),
		y: positiveInteger(gridValue.y, "fields.Grid.y"),
		z: positiveInteger(gridValue.z, "fields.Grid.z"),
		spacing: finiteNumber(gridValue.spacing, "fields.Grid.spacing"),
	};
	const cells = Grid.x * Grid.y * Grid.z;

	return {
		Grid,
		Density: numberArray(fields.Density, cells, "fields.Density"),
		Momentum: numberArray(fields.Momentum, cells * 3, "fields.Momentum"),
		InternalEnergy: numberArray(
			fields.InternalEnergy,
			cells,
			"fields.InternalEnergy",
		),
		WaveReal: numberArray(fields.WaveReal, cells, "fields.WaveReal"),
		WaveImaginary: numberArray(
			fields.WaveImaginary,
			cells,
			"fields.WaveImaginary",
		),
	};
};

export const decodeParticles = (value: unknown): FluidParticle[] => {
	const envelope = objectValue(value, "fluid envelope") as FluidEnvelope;

	if (!Array.isArray(envelope.particles)) {
		throw new Error("particles must be an array");
	}

	return envelope.particles as FluidParticle[];
};
