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
	direction: string;
	forwardReturn: number;
	horizon: number;
};

export type TerminalPhaseStatus = {
	ready: boolean;
	reason: string;
};

/*
phaseColumnsFromScan groups the system's rotation by angle, preserving the
backend rank order so the geodesic can be drawn without re-sorting.
*/
export const phaseColumnsFromScan = (
	scan: TerminalPhaseResponse[],
): TerminalPhaseResponse[][] => {
	const columns = new Map<number, TerminalPhaseResponse[]>();

	for (const response of scan) {
		const column = columns.get(response.angle);

		if (column === undefined) {
			columns.set(response.angle, [response]);
			continue;
		}

		column.push(response);
	}

	return [...columns.entries()]
		.sort((left, right) => left[0] - right[0])
		.map(([, column]) => column);
};

/*
phaseLeadersFromScan keeps the most constructive match at each angle — the
envelope the dial already drew — without discarding the rest of the rank.
*/
export const phaseLeadersFromScan = (
	scan: TerminalPhaseResponse[],
): TerminalPhaseResponse[] =>
	phaseColumnsFromScan(scan).flatMap((column) => {
		const leader = column[0];

		if (leader === undefined) {
			return [];
		}

		return [leader];
	});

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
finiteNumber keeps only finite numeric wire values.
*/
export const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const stringValue = (value: unknown): string =>
	typeof value === "string" ? value.trim() : "";

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
		const linewidth = finiteNumber(record.linewidth) ?? 0;

		if (omega === null || real === null || imaginary === null) {
			return [];
		}

		return [{ omega, real, imaginary, linewidth }];
	});

/*
terminalPhaseScanFromFrame reads signed corpus responses over the backend's
mode-derived angular path together with the realized direction that owns each
sector.
*/
export const terminalPhaseScanFromFrame = (
	frame: Record<string, unknown> | null | undefined,
): TerminalPhaseResponse[] =>
	recordArray(frame?.phaseScan).flatMap((record) => {
		const angle = finiteNumber(record.angle);
		const similarity = finiteNumber(record.similarity);
		const observedAt = stringValue(record.observedAt);
		const outcome = asRecord(record.outcome);
		const direction = stringValue(outcome?.direction);
		const forwardReturn = finiteNumber(outcome?.return);
		const horizon = finiteNumber(outcome?.horizon);

		if (
			angle === null ||
			similarity === null ||
			observedAt === "" ||
			direction === "" ||
			forwardReturn === null ||
			horizon === null
		) {
			return [];
		}

		return [
			{
				angle,
				similarity,
				observedAt,
				outcome: { direction, forwardReturn, horizon },
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
fluidGridDimensions reads explicit backend grid metadata for display labels.
*/
export const fluidGridDimensions = (
	frame: Record<string, unknown> | null | undefined,
): { columns: number; rows: number } => {
	const grid = asRecord(frame?.grid);
	const gridX = finiteNumber(grid?.x);
	const gridZ = finiteNumber(grid?.z);

	return {
		columns: gridX !== null && gridX > 0 ? gridX : 0,
		rows: gridZ !== null && gridZ > 0 ? gridZ : 0,
	};
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
