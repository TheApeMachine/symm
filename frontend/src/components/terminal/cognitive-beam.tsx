import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import type { CognitiveReading } from "#/collections/types";
import { useDecisionsScopeSymbol } from "#/components/terminal/decision-side";
import { Component } from "#/components/ui/component";
import { meterTrackVariants } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";

const isConcreteSymbol = (symbol: string | undefined): symbol is string =>
	symbol !== undefined && symbol !== "" && symbol !== "stream";

/*
cognitiveScopes lists symbols that currently own a cognitive reading.
*/
export const cognitiveScopes = (readings: CognitiveReading[]): string[] =>
	[
		...new Set(
			readings
				.map((reading) => reading.symbol || reading.scope || "")
				.filter((scope) => scope !== ""),
		),
	].sort();

export const cognitiveReadingFor = (
	readings: CognitiveReading[] | Record<string, CognitiveReading>,
	symbol?: string,
): CognitiveReading | null => {
	const rows = Array.isArray(readings) ? readings : Object.values(readings);

	if (isConcreteSymbol(symbol)) {
		return rows.find((reading) => reading.symbol === symbol) ?? null;
	}

	const [scope] = cognitiveScopes(rows);

	return scope === undefined
		? null
		: (rows.find((reading) => reading.symbol === scope) ?? null);
};

/*
The meters read straight off the classifier's own numbers. Entropy is shown
against the threshold it is judged by rather than as a bare bit count, because
the only thing that matters about it is which side of that line it sits on —
data-set-scale="above-threshold" colours it from the same frame that carries it.
*/
const METERS = [
	{
		key: "confidence",
		label: "Class confidence",
		path: "confidence",
		format: ".1%",
		variant: "info",
	},
	{
		key: "lookahead",
		label: "Lookahead beam",
		path: "lookaheadScore",
		format: ".3f",
		variant: "warning",
	},
	{
		key: "contrast",
		label: "Class contrast",
		path: "contrast",
		format: ".3f",
		variant: "success",
	},
] as const;

/*
CognitiveBeam reports the DMT beam diagnostics for the candidate the decision
rail has selected, falling back to the dashboard focus.

Cognition is published as a symbol-keyed map, so the reading is selected by
naming the symbol: a miss is a real answer — the classifier has published
nothing for it — and the panel says so rather than showing another symbol's
beam under this one's name.
*/
export const CognitiveBeam = () => {
	const scope = useDecisionsScopeSymbol();
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const symbol = isConcreteSymbol(scope) ? scope : focusSymbol;

	if (!isConcreteSymbol(symbol)) {
		return (
			<Panel className="mt-3.5">
				<div className="font-semibold text-[12px] text-(--f1)">
					Cognitive beam
				</div>
				<div className="mt-2 font-mono text-[9.5px] text-(--f4)">
					waiting for cognitive frame
				</div>
			</Panel>
		);
	}

	return (
		<Component registerKey="cognition" select={symbol}>
			{({ ref }) => (
				<div ref={ref} className="mt-3.5">
					<Panel>
						<div className="flex items-center justify-between">
							<span className="font-semibold text-[12px] text-(--f1)">
								Cognitive beam
							</span>
							<span
								data-paint="cohort"
								data-paint-absent="—"
								className="rounded-full border border-(--line2) px-2 py-px font-mono text-[9px] text-(--info)"
							/>
						</div>
						<div className="mt-2 font-mono text-[9.5px] text-(--f4)">
							DMT sequence
						</div>
						<div
							data-paint="sequence"
							data-paint-empty="no sequence yet"
							data-paint-absent="waiting for cognitive frame"
							className="mt-1 line-clamp-3 wrap-break-word rounded-sm border border-(--line) bg-(--bg) p-1.5 font-mono text-[10px] text-(--f2)"
						/>

						<div className="mt-3 flex flex-col gap-2.5">
							<div>
								<div className="mb-1 flex justify-between font-mono text-[10px]">
									<span className="text-(--f3)">Entropy gate</span>
									<span
										data-paint="entropyBits"
										data-paint-format=".3f"
										data-set="entropyBits"
										data-set-scale="above-threshold"
										data-set-threshold="entropyThreshold"
										data-target="style.color"
										data-paint-absent="—"
									/>
								</div>
								<div className="font-mono text-[9px] text-(--f4)">
									threshold{" "}
									<span
										data-paint="entropyThreshold"
										data-paint-format=".3f"
										data-paint-absent="—"
									/>
								</div>
							</div>

							{METERS.map((meter) => (
								<div key={meter.key}>
									<div className="mb-1 flex justify-between font-mono text-[10px]">
										<span className="text-(--f3)">{meter.label}</span>
										<span
											data-paint={meter.path}
											data-paint-format={meter.format}
											data-paint-absent="—"
											className="text-(--f1)"
										/>
									</div>
									<div
										className={meterTrackVariants({
											variant: meter.variant,
											size: "s",
										})}
									>
										<div
											data-set={meter.path}
											data-target="style.--beam"
											className="h-full bg-(--meter-tone)"
											style={{
												width: "clamp(0%, calc(var(--beam, 0) * 100%), 100%)",
											}}
										/>
									</div>
								</div>
							))}
						</div>

						<div className="mt-3 grid grid-cols-2 gap-1.5 font-mono text-[10px]">
							<div className="flex justify-between">
								<span className="text-(--f4)">winner</span>
								<span
									data-paint="winner"
									data-paint-empty="pending"
									data-paint-absent="—"
									className="text-(--acc)"
								/>
							</div>
							<div className="flex justify-between">
								<span className="text-(--f4)">paths</span>
								<span
									data-paint="lookaheadPaths"
									data-paint-absent="—"
									className="text-(--f1)"
								/>
							</div>
						</div>
					</Panel>
				</div>
			)}
		</Component>
	);
};
