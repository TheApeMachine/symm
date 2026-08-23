import type {
	HindsightRecommendation,
	HindsightReport,
	HindsightRootCause,
} from "#/collections/app";

export const formatHindsightCategory = (category: string): string =>
	category
		.split("_")
		.filter((part) => part !== "")
		.map((part) => `${part[0]?.toUpperCase() ?? ""}${part.slice(1)}`)
		.join(" ");

/*
The backend owns hindsight arithmetic and experiment ranking. These selectors
only expose its published verdicts; they deliberately do not reconstruct or
rescore candidates from raw signal values in the browser.
*/
export const hindsightRealizedPct = (report: HindsightReport): number =>
	report.realizedPct ?? 0;

export const hindsightValueCaptureRate = (report: HindsightReport): number =>
	report.valueCaptureRate ?? 0;

export const hindsightLegCaptureRate = (report: HindsightReport): number =>
	report.legCaptureRate ?? 0;

export const hindsightDiagnosticCoverage = (
	report: HindsightReport,
): number => report.diagnosticCoverage ?? 0;

export const rankHindsightRecommendations = (
	report: HindsightReport,
): HindsightRecommendation[] => report.recommendations ?? [];

export const rankHindsightRootCauses = (
	report: HindsightReport,
): HindsightRootCause[] => report.rootCauses ?? [];
