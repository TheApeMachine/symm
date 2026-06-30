import type { TerminalKernel } from "#/components/terminal/model";

type ReadingsState = Record<string, Record<string, unknown>>;

const numberFrom = (output: Record<string, unknown>, key: string): number => {
	const nested = output[key];

	return typeof nested === "number" ? nested : 0;
};

/*
kernelsForFocus projects the latest measurement per origin (for the most active
symbol that has data) into the TerminalKernel shape the Decision Tree and
Allocation surfaces consume. confidence/surprise come straight off the backend
measurement output — no client-side scoring, just unit→percent scaling.

readings is the measurementsStore state: origin → symbol → frame. When a focus
symbol is supplied its readings are used; otherwise the first symbol each origin
has data for is taken, so a kernel set exists even before a symbol is focused.
*/
export const kernelsForFocus = (
	readings: ReadingsState,
	focusSymbol?: string,
): TerminalKernel[] => {
	const kernels: TerminalKernel[] = [];

	for (const origin of Object.keys(readings)) {
		const bySymbol = readings[origin] as
			| Record<string, Record<string, unknown>>
			| undefined;

		if (bySymbol === undefined) {
			continue;
		}

		const symbol =
			focusSymbol !== undefined && bySymbol[focusSymbol] !== undefined
				? focusSymbol
				: Object.keys(bySymbol)[0];

		if (symbol === undefined) {
			continue;
		}

		const frame = bySymbol[symbol];
		const output = (frame?.output ?? {}) as Record<string, unknown>;
		const confidence = numberFrom(output, "confidence");
		const surprise = numberFrom(output, "surprise");

		kernels.push({
			source: origin,
			confidencePercent: Math.round(confidence * 100),
			surprisePercent: Math.round(Math.min(surprise, 1) * 100),
		});
	}

	return kernels;
};
