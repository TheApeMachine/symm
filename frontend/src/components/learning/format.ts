/*
Display formatting for the learning surface. These helpers only choose how a
measured number is written; none of them substitutes a value, clamps a range
or supplies a fallback reading. An absent measurement is written as absent.
*/

export const amount = (value: number) =>
	value.toLocaleString(undefined, { maximumFractionDigits: 4 });

/*
basis writes a return as basis points. Returns here are fractions of a lane's
starting capital, and the interesting ones are far smaller than a percent.
*/
export const basis = (value: number) => `${(10000 * value).toFixed(1)} bp`;

export const percent = (value: number) => `${(100 * value).toFixed(1)}%`;

export const clock = (value: string) =>
	value.startsWith("0001-")
		? "not valued"
		: new Date(value).toLocaleTimeString();

/* duration writes a nanosecond window in the largest unit that stays readable. */
export const duration = (nanoseconds: number) => {
	if (!Number.isFinite(nanoseconds) || nanoseconds <= 0) {
		return "unmeasured";
	}

	const seconds = nanoseconds / 1e9;

	if (seconds < 1) {
		return `${(seconds * 1000).toFixed(0)} ms`;
	}

	if (seconds < 90) {
		return `${seconds.toFixed(1)} s`;
	}

	return `${(seconds / 60).toFixed(1)} min`;
};

/*
action writes one action with its quantity refinement. Power is a bisection
depth, not an allocation percentage: power 0 is the whole executable range,
each further power halves it.
*/
export const action = (kind: string, power: number, reduce: boolean) => {
	if (!kind || kind === "hold") {
		return "wait";
	}

	return `${kind}${reduce ? " ↓" : ""} ·1/${2 ** power}`;
};

/* rational formats exact account strings for display; the journal retains exact fractions. */
export const rational = (value: string | undefined) => {
	if (!value) return "unavailable";
	const [numerator, denominator] = value.split("/");
	return amount(
		Number(numerator) / (denominator === undefined ? 1 : Number(denominator)),
	);
};
