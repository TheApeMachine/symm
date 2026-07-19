import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
import { appStore } from "#/collections/app";
import type { CognitiveReading } from "#/collections/types";
import {
	formatBeamSequence,
	formatEntropyGate,
} from "#/components/terminal/cognitive-entropy";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { getWorker } from "#/providers/websocket";
import { meterTrackVariants } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import type { Variant } from "@/components/ui/types";

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

export type CognitiveBeamModel = {
	sequenceTitle: string;
	cohort: string;
	sequence: string;
	winner: string;
	paths: string;
	meters: Array<{
		label: string;
		value: string;
		percent: number;
		variant: Variant;
	}>;
};

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

export const cognitiveBeamModel = (
	reading: CognitiveReading | null,
): CognitiveBeamModel | null => {
	if (reading === null) {
		return null;
	}

	const entropyBits =
		typeof reading.entropyBits === "number" &&
		Number.isFinite(reading.entropyBits)
			? reading.entropyBits
			: 0;
	const entropyThreshold =
		typeof reading.entropyThreshold === "number" &&
		Number.isFinite(reading.entropyThreshold)
			? reading.entropyThreshold
			: 0;
	const entropyGate = formatEntropyGate(entropyBits, entropyThreshold);
	const sequence = formatBeamSequence(reading.sequence || "waiting");
	const confidence = Math.min(
		1,
		Math.max(
			0,
			typeof reading.classConfidence === "number" &&
				Number.isFinite(reading.classConfidence)
				? reading.classConfidence
				: 0,
		),
	);
	const lookahead = Math.min(
		1,
		Math.max(
			0,
			typeof reading.lookaheadScore === "number" &&
				Number.isFinite(reading.lookaheadScore)
				? reading.lookaheadScore
				: 0,
		),
	);
	const paths =
		(typeof reading.lookaheadPaths === "number" &&
		Number.isFinite(reading.lookaheadPaths)
			? reading.lookaheadPaths
			: null) ??
		(typeof reading.prewarmPaths === "number" &&
		Number.isFinite(reading.prewarmPaths)
			? reading.prewarmPaths
			: 0);

	return {
		cohort: String(reading.regimeCohort),
		sequence: sequence.preview,
		sequenceTitle: sequence.title,
		winner: reading.winnerClass || "pending",
		paths: String(Math.round(paths)),
		meters: [
			{
				label: "Entropy gate",
				value: entropyGate.value,
				percent: entropyGate.percent,
				variant: entropyGate.ungated ? "disabled" : "success",
			},
			{
				label: "Class confidence",
				value: `${Math.round(confidence * 100)}%`,
				percent: confidence * 100,
				variant: "info",
			},
			{
				label: "Lookahead beam",
				value: lookahead.toFixed(3),
				percent: lookahead * 100,
				variant: "warning",
			},
		],
	};
};

const setText = (node: Element | null | undefined, value: string): void => {
	if (node instanceof HTMLElement) {
		node.textContent = value;
	}
};

const METER_KEYS = ["entropy", "confidence", "lookahead"] as const;

/*
paintCognitiveBeam writes live DMT beam readings into a mounted shell so
websocket cadence never re-renders the CognitiveBeam React tree.
*/
export const paintCognitiveBeam = (
	root: HTMLElement | null,
	model: CognitiveBeamModel | null,
): void => {
	if (root === null) {
		return;
	}

	const waiting = root.querySelector("[data-beam='waiting']");
	const panel = root.querySelector("[data-beam='panel']");

	if (waiting instanceof HTMLElement) {
		waiting.style.display = model === null ? "" : "none";
	}

	if (panel instanceof HTMLElement) {
		panel.style.display = model === null ? "none" : "";
	}

	if (model === null) {
		return;
	}

	setText(root.querySelector("[data-beam='cohort']"), `cohort ${model.cohort}`);
	setText(root.querySelector("[data-beam='sequence']"), model.sequence);

	const sequence = root.querySelector("[data-beam='sequence']");

	if (sequence instanceof HTMLElement) {
		sequence.title = model.sequenceTitle;
	}

	setText(root.querySelector("[data-beam='winner']"), model.winner);
	setText(root.querySelector("[data-beam='paths']"), model.paths);

	for (const [index, key] of METER_KEYS.entries()) {
		const meter = model.meters[index];
		const row = root.querySelector(`[data-beam-meter='${key}']`);

		if (!(row instanceof HTMLElement) || meter === undefined) {
			continue;
		}

		setText(row.querySelector("[data-beam='meter-value']"), meter.value);

		const track = row.querySelector("[data-beam='meter-track']");
		const fill = row.querySelector("[data-beam='meter-fill']");

		if (track instanceof HTMLElement) {
			track.className = meterTrackVariants({
				variant: meter.variant,
				size: "s",
			});
		}

		if (fill instanceof HTMLElement) {
			fill.style.width = `${Math.max(0, Math.min(100, meter.percent))}%`;
		}
	}
};

/*
CognitiveBeam paints DMT beam diagnostics from the cognitive store without
React reconciliation on each websocket tick.
*/
export const CognitiveBeam = ({ symbol }: { symbol?: string }) => {
	const rootRef = useRef<HTMLDivElement>(null);
	const online = useSelector(appStore, (state) => state.online);

	useDirectStorePaint(
		getWorker(),
		[{ store: "cognitive", key: "" }],
		(buffers) =>
			paintCognitiveBeam(
				rootRef.current,
				cognitiveBeamModel(
					cognitiveReadingFor(
						(buffers["cognitive:"] ?? []) as CognitiveReading[],
						symbol,
					),
				),
			),
		[online, symbol],
	);

	return (
		<div ref={rootRef} className="mt-3.5">
			<Panel data-beam="waiting">
				<div className="font-semibold text-[12px] text-(--f1)">
					Cognitive beam
				</div>
				<div className="mt-2 font-mono text-[9.5px] text-(--f4)">
					waiting for cognitive frame
				</div>
			</Panel>

			<Panel data-beam="panel" style={{ display: "none" }}>
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">
						Cognitive beam
					</span>
					<span
						data-beam="cohort"
						className="rounded-full border border-(--line2) px-2 py-px font-mono text-[9px] text-(--info)"
					/>
				</div>
				<div className="mt-2 font-mono text-[9.5px] text-(--f4)">
					DMT sequence
				</div>
				<div
					data-beam="sequence"
					className="mt-1 line-clamp-3 wrap-break-word rounded-sm border border-(--line) bg-(--bg) p-1.5 font-mono text-[10px] text-(--f2)"
				/>

				<div className="mt-3 flex flex-col gap-2.5">
					{(
						[
							["entropy", "Entropy gate"],
							["confidence", "Class confidence"],
							["lookahead", "Lookahead beam"],
						] as const
					).map(([key, label]) => (
						<div key={key} data-beam-meter={key}>
							<div className="mb-1 flex justify-between font-mono text-[10px]">
								<span className="text-(--f3)">{label}</span>
								<span data-beam="meter-value" className="text-(--f1)" />
							</div>
							<div
								data-beam="meter-track"
								className={meterTrackVariants({
									variant: "info",
									size: "s",
								})}
							>
								<div
									data-beam="meter-fill"
									className="h-full bg-(--meter-tone)"
									style={{ width: "0%" }}
								/>
							</div>
						</div>
					))}
				</div>

				<div className="mt-3 grid grid-cols-2 gap-1.5 font-mono text-[10px]">
					<div className="flex justify-between">
						<span className="text-(--f4)">winner</span>
						<span data-beam="winner" className="text-(--acc)" />
					</div>
					<div className="flex justify-between">
						<span className="text-(--f4)">paths</span>
						<span data-beam="paths" className="text-(--f1)" />
					</div>
				</div>
			</Panel>
		</div>
	);
};
