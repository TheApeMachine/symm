import type { FluidFieldLayer } from "#/collections/terminal";
import {
	isFluidFieldMatrix,
	resolveFluidDisplayLattice,
} from "#/components/terminal/fluid-field";
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
	outcome: TerminalPhaseOutcome;
};

export type TerminalPhaseOutcome = {
	symbol: string;
	className: string;
	confidence: number;
	ambiguous: boolean;
	cohort: number;
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

const SHARED_FIELD_KEYS = [
	"rho",
	"psiMag2",
	"guidanceVelX",
	"guidanceVelZ",
	"wave",
] as const;

/*
withSharedManifoldField overlays the batch's single shared Sensorium lattices
onto the focused symbol row. Backend wireManifold keeps ρ/|ψ|² on one carrier
only; the pilot-wave paint still needs those lattices under every focus.
*/
export const withSharedManifoldField = <
	T extends Record<string, unknown> & { symbol?: string },
>(
	focus: T | null | undefined,
	batch: readonly T[],
): T | null => {
	if (focus == null) {
		return null;
	}

	if (isFluidFieldMatrix(frameAuxMatrix(focus, "rho"))) {
		return focus;
	}

	const carrier = batch.find((frame) =>
		isFluidFieldMatrix(frameAuxMatrix(frame, "rho")),
	);

	if (carrier == null) {
		return focus;
	}

	const inherited: Record<string, unknown> = { ...focus };

	for (const key of SHARED_FIELD_KEYS) {
		if (carrier[key] !== undefined) {
			inherited[key] = carrier[key];
		}
	}

	return inherited as T;
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
	const cellZ = finiteNumber(record.cell_z);
	const phase = finiteNumber(record.phase);
	const amplitude = finiteNumber(record.amplitude);
	const velX = finiteNumber(record.vel_x);
	const velZ = finiteNumber(record.vel_z);

	if (
		cellX === null ||
		cellZ === null ||
		phase === null ||
		amplitude === null ||
		velX === null ||
		velZ === null
	) {
		return null;
	}

	const cellY = finiteNumber(record.cell_y) ?? 0;
	const omega = finiteNumber(record.omega) ?? 0;
	const heat = finiteNumber(record.heat) ?? 0;
	const velY = finiteNumber(record.vel_y) ?? 0;

	return {
		source: stringValue(record.source),
		role: stringValue(record.role) || "particle",
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
mode-derived angular path together with the DMT outcome that owns each sector.
*/
export const terminalPhaseScanFromFrame = (
	frame: Record<string, unknown> | null | undefined,
): TerminalPhaseResponse[] =>
	recordArray(frame?.phaseScan).flatMap((record) => {
		const angle = finiteNumber(record.angle);
		const similarity = finiteNumber(record.similarity);
		const observedAt = stringValue(record.observedAt);
		const outcome = asRecord(record.outcome);
		const symbol = stringValue(outcome?.symbol);
		const className = stringValue(outcome?.class);
		const confidence = finiteNumber(outcome?.confidence);
		const cohort = finiteNumber(outcome?.cohort);

		if (
			angle === null ||
			similarity === null ||
			observedAt === "" ||
			symbol === "" ||
			className === "" ||
			confidence === null ||
			cohort === null ||
			typeof outcome?.ambiguous !== "boolean"
		) {
			return [];
		}

		return [
			{
				angle,
				similarity,
				observedAt,
				outcome: {
					symbol,
					className,
					confidence,
					ambiguous: outcome.ambiguous,
					cohort,
				},
			},
		];
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
