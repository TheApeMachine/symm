/*
DomainError is a typed failure for undefined numeric statistics.
Callers must handle it explicitly instead of substituting 0, NaN, or epsilon.
*/
export type DomainErrorCode =
	| "empty_sample"
	| "insufficient_sample"
	| "zero_denominator"
	| "non_positive_base";

/*
DomainError carries a stable code so UI and tests can branch without string matching.
*/
export class DomainError extends Error {
	readonly code: DomainErrorCode;

	constructor(code: DomainErrorCode, message: string) {
		super(message);
		this.name = "DomainError";
		this.code = code;
	}
}

/*
requirePositiveLength rejects empty collections before a mean or layout division.
*/
export const requirePositiveLength = (length: number, label: string): void => {
	if (length < 1) {
		throw new DomainError(
			"empty_sample",
			`${label}: sample length must be at least 1`,
		);
	}
};

/*
requireSampleSize rejects collections that are too small for an n-based estimator.
*/
export const requireSampleSize = (
	n: number,
	minimum: number,
	label: string,
): void => {
	if (n < minimum) {
		throw new DomainError(
			"insufficient_sample",
			`${label}: sample length must be at least ${minimum}`,
		);
	}
};

/*
requireNonZero evaluates a denominator once and rejects zero before division.
*/
export const requireNonZero = (value: number, label: string): number => {
	if (value === 0 || !Number.isFinite(value)) {
		throw new DomainError(
			"zero_denominator",
			`${label}: denominator must be finite and nonzero`,
		);
	}

	return value;
};

/*
requirePositive rejects non-positive capital, price, or mass bases.
*/
export const requirePositive = (value: number, label: string): number => {
	if (!(value > 0) || !Number.isFinite(value)) {
		throw new DomainError(
			"non_positive_base",
			`${label}: value must be finite and strictly positive`,
		);
	}

	return value;
};
