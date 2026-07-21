import {
	isFluidFieldMatrix,
	resolveFluidDisplayLattice,
} from "#/components/terminal/fluid-field";
import type { FluidFieldLayer } from "#/collections/terminal";
import type { TerminalFluidParticle } from "#/components/terminal/fluid-particles";

const asRecord = (value: unknown): Record<string, unknown> | null =>
	value !== null && typeof value === "object" && !Array.isArray(value)
		? (value as Record<string, unknown>)
		: null;

const numberArray = (value: unknown): number[] =>
	Array.isArray(value)
		? value.filter((item): item is number => typeof item === "number")
		: [];

const recordArray = (value: unknown): Record<string, unknown>[] =>
	Array.isArray(value)
		? value.flatMap((item) => {
				const record = asRecord(item);
				return record === null ? [] : [record];
			})
		: [];

export type TerminalWaveMode = {
	omega: number;
	real: number;
	imaginary: number;
	linewidth: number;
};

export type TerminalPhaseResponse = {
	angle: number;
	similarity: number;
	observedAt: string;
};

export type TerminalPhaseStatus = {
	ready: boolean;
	reason: string;
};

const numberMatrix = (value: unknown): number[][] =>
	Array.isArray(value)
		? value.map((row) => numberArray(row)).filter((row) => row.length > 0)
		: [];

const frameOutput = (
	frame: Record<string, unknown> | null | undefined,
): Record<string, unknown> | null => asRecord(frame?.output);

/*
frameReading resolves nested manifold/resonance reading objects from either the
frame root or its output envelope.
*/
export const frameReading = (
	frame: Record<string, unknown> | null | undefined,
): Record<string, unknown> | null =>
	asRecord(frame?.reading) ?? asRecord(frameOutput(frame)?.reading);

/*
frameMatrix extracts a drawable numeric lattice from a manifold-shaped frame.
*/
export const frameMatrix = (
	frame: Record<string, unknown> | null | undefined,
): number[][] => {
	const output = frameOutput(frame);
	const reading = frameReading(frame);

	for (const value of [
		frame?.rho,
		output?.rho,
		frame?.matrix,
		output?.matrix,
		frame?.grid,
		output?.grid,
	]) {
		const matrix = numberMatrix(value);

		if (matrix.length > 0) {
			return matrix;
		}
	}

	for (const value of [
		frame?.state,
		output?.state,
		frame?.values,
		output?.values,
	]) {
		const row = numberArray(value);

		if (row.length > 0) {
			return [row];
		}
	}

	const scalarRow = [
		frame?.bidTouchDensity,
		frame?.askTouchDensity,
		frame?.pressureGradX,
		frame?.pressureGradY,
		frame?.pressureGradZ,
		frame?.divergence,
		frame?.coherenceMag2,
		frame?.guidanceSpeed,
		frame?.stressAnisotropy,
		reading?.pressureGradX,
		reading?.pressureGradY,
		reading?.pressureGradZ,
		reading?.divergence,
		reading?.coherenceMag2,
		reading?.guidanceSpeed,
		reading?.viscosityProxy,
	].filter(
		(value): value is number =>
			typeof value === "number" && Number.isFinite(value),
	);

	if (scalarRow.length > 0) {
		return [scalarRow];
	}

	return [];
};

/*
frameAuxMatrix reads a named matrix field from a frame or its output envelope.
*/
export const frameAuxMatrix = (
	frame: Record<string, unknown> | null | undefined,
	field: string,
): number[][] => {
	const output = frameOutput(frame);

	for (const value of [frame?.[field], output?.[field]]) {
		const matrix = numberMatrix(value);

		if (matrix.length > 0) {
			return matrix;
		}
	}

	return [];
};

/*
finiteNumber keeps only finite numeric wire values.
*/
export const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const stringValue = (value: unknown): string =>
	typeof value === "string" ? value.trim() : "";

const terminalFluidParticleFromRecord = (
	record: Record<string, unknown>,
): TerminalFluidParticle | null => {
	const cellX = finiteNumber(record.cell_x);
	const cellY = finiteNumber(record.cell_y);
	const cellZ = finiteNumber(record.cell_z);
	const phase = finiteNumber(record.phase);
	const omega = finiteNumber(record.omega);
	const amplitude = finiteNumber(record.amplitude);
	const heat = finiteNumber(record.heat);
	const velX = finiteNumber(record.vel_x);
	const velY = finiteNumber(record.vel_y);
	const velZ = finiteNumber(record.vel_z);

	if (
		cellX === null ||
		cellY === null ||
		cellZ === null ||
		phase === null ||
		omega === null ||
		amplitude === null ||
		heat === null ||
		velX === null ||
		velY === null ||
		velZ === null
	) {
		return null;
	}

	return {
		source: stringValue(record.source),
		role: stringValue(record.role),
		cellX,
		cellY,
		cellZ,
		phase,
		omega,
		amplitude,
		heat,
		velX,
		velY,
		velZ,
		speed: finiteNumber(record.speed) ?? Math.hypot(velX, velY, velZ),
	};
};

/*
terminalFluidParticlesFromFrame maps wire particles onto the fluid canvas model.
*/
export const terminalFluidParticlesFromFrame = (
	frame: Record<string, unknown> | null | undefined,
): TerminalFluidParticle[] =>
	recordArray(frame?.particles).flatMap((record) => {
		const particle = terminalFluidParticleFromRecord(record);
		return particle === null ? [] : [particle];
	});

/*
terminalWaveModesFromFrame reads the resident complex omega field without
collapsing phase into magnitude before the phase-dial renderer sees it.
*/
export const terminalWaveModesFromFrame = (
	frame: Record<string, unknown> | null | undefined,
): TerminalWaveMode[] =>
	recordArray(frame?.wave).flatMap((record) => {
		const omega = finiteNumber(record.omega);
		const real = finiteNumber(record.real);
		const imaginary = finiteNumber(record.imaginary);
		const linewidth = finiteNumber(record.linewidth);

		if (
			omega === null ||
			real === null ||
			imaginary === null ||
			linewidth === null
		) {
			return [];
		}

		return [{ omega, real, imaginary, linewidth }];
	});

/*
terminalPhaseScanFromFrame reads signed corpus responses over the backend's
mode-derived angular path so destructive interference remains visible.
*/
export const terminalPhaseScanFromFrame = (
	frame: Record<string, unknown> | null | undefined,
): TerminalPhaseResponse[] =>
	recordArray(frame?.phaseScan).flatMap((record) => {
		const angle = finiteNumber(record.angle);
		const similarity = finiteNumber(record.similarity);
		const observedAt = stringValue(record.observedAt);

		if (angle === null || similarity === null || observedAt === "") {
			return [];
		}

		return [{ angle, similarity, observedAt }];
	});

/*
terminalPhaseStatusFromFrame preserves the backend's explicit distinction
between a usable historical scan and a resident wave still awaiting context.
*/
export const terminalPhaseStatusFromFrame = (
	frame: Record<string, unknown> | null | undefined,
): TerminalPhaseStatus => ({
	ready: frame?.phaseReady === true,
	reason: stringValue(frame?.phaseReason),
});

/*
fluidGridDimensions prefers explicit grid metadata, then falls back to matrix shape.
*/
export const fluidGridDimensions = (
	frame: Record<string, unknown> | null | undefined,
	matrix: number[][],
): { columns: number; rows: number } => {
	const grid = asRecord(frame?.grid);
	const gridX = finiteNumber(grid?.x);
	const gridZ = finiteNumber(grid?.z);
	const columns =
		gridX !== null && gridX > 0 ? gridX : (matrix[0]?.length ?? 0);
	const rows = gridZ !== null && gridZ > 0 ? gridZ : matrix.length;

	return { columns, rows };
};

/*
terminalResonanceLayerMatrixFromFrame builds a heat matrix from resonance layers.
*/
export const terminalResonanceLayerMatrixFromFrame = (
	frame: Record<string, unknown> | null,
): number[][] => {
	const latent = numberArray(frame?.latent);
	const modes = numberArray([frame?.flow, frame?.stress, frame?.coupling]);
	const energy = numberArray([frame?.baseline, frame?.energy, frame?.surprise]);

	return [latent, modes, energy].filter((row) => row.length > 0);
};

export const terminalFluidMatrixFromFrame = frameMatrix;

/*
terminalFluidPsiMatrixFromFrame reads the ψ magnitude lattice from a manifold frame.
*/
export const terminalFluidPsiMatrixFromFrame = (
	frame: Record<string, unknown> | null | undefined,
): number[][] => frameAuxMatrix(frame, "psiMag2");

/*
terminalFluidDisplayLatticeFromFrame selects the requested physical field layer
without combining gas density and coherence into one scalar lattice.
*/
export const terminalFluidDisplayLatticeFromFrame = (
	frame: Record<string, unknown> | null | undefined,
	layer: FluidFieldLayer = "Composite",
): number[][] => {
	const rho = frameAuxMatrix(frame, "rho");
	const psiMag2 = frameAuxMatrix(frame, "psiMag2");
	const lattice = isFluidFieldMatrix(rho) ? rho : [];

	return resolveFluidDisplayLattice(lattice, psiMag2, layer);
};
