import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { useDecisionsScopeSymbol } from "#/components/terminal/decision-side";
import { Typography } from "@/components/ui/typography";
import { meterTrackVariants } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import { cognitionStore, useSubscribe } from "#/providers/ws-stores";

import type { CognitiveReading } from "#/collections/types";

const isConcreteSymbol = (symbol: string | undefined): symbol is string =>
	symbol !== undefined && symbol !== "" && symbol !== "stream";

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

const METERS = [
	{ key: "confidence", label: "Class confidence", path: "confidence", variant: "info" },
	{ key: "lookahead", label: "Lookahead beam", path: "lookaheadScore", variant: "warning" },
	{ key: "contrast", label: "Class contrast", path: "contrast", variant: "success" },
] as const;

export const CognitiveBeam = () => {
	const scope = useDecisionsScopeSymbol();
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const symbol = isConcreteSymbol(scope) ? scope : focusSymbol;

	const root = useSubscribe(cognitionStore, (state) => {
		const row = state.cognition[symbol]?.latest() ?? null;

		const set = (q: string, value: string) => {
			const els = root.current?.querySelectorAll<HTMLElement>(`[data-f=${q}]`);

			els?.forEach((el) => {
				if (el instanceof HTMLElement) {
					el.textContent = value;
				}
			});
		};

		set("cohort", row?.cohort === undefined ? "—" : String(row.cohort));
		set("sequence", row?.sequence === undefined || row.sequence === "" ? "no sequence yet" : String(row.sequence));
		set("entropy", row?.entropyBits === undefined ? "—" : row.entropyBits.toFixed(3));
		set("paths", row?.lookaheadPaths === undefined ? "—" : String(row.lookaheadPaths));
		set("winner", row?.winner === undefined ? "pending" : String(row.winner));
		set("candidate", row?.candidateWinner === undefined ? "pending" : String(row.candidateWinner));
		set("held", row?.stateHeld === undefined ? "false" : String(row.stateHeld));
		set("switchConf", row?.switchConfidence === undefined ? "—" : `${(row.switchConfidence * 100).toFixed(1)}%`);
		set("switchThresh", row?.switchThreshold === undefined ? "—" : `${(row.switchThreshold * 100).toFixed(1)}%`);

		for (const meter of METERS) {
			const value = row?.[meter.path as keyof typeof row] as number | undefined;
			set(meter.key, value === undefined ? "—" : value.toFixed(3));

			const bar = root.current?.querySelector<HTMLElement>(`[data-meter="${meter.key}"]`);

			if (bar instanceof HTMLElement && typeof value === "number") {
				bar.style.width = `clamp(0%, calc(${Math.min(1, Math.max(0, value))} * 100%), 100%)`;
			}
		}
	}, [symbol]);

	if (!isConcreteSymbol(symbol)) {
		return (
			<Panel className="mt-3.5">
				<Typography.Label size="lg" tone="f1" className="block">Cognitive beam</Typography.Label>
				<div className="mt-2 font-mono text-[9.5px] text-(--f4)">waiting for cognitive frame</div>
			</Panel>
		);
	}

	return (
		<div ref={root} className="mt-3.5">
			<Panel>
				<div className="flex items-center justify-between">
					<Typography.Label size="lg" tone="f1">Cognitive beam</Typography.Label>
					<span data-f="cohort" className="rounded-full border border-(--line2) px-2 py-px font-mono text-[9px] text-(--info)" />
				</div>
				<div className="mt-2 font-mono text-[9.5px] text-(--f4)">DMT sequence</div>
				<div data-f="sequence" className="mt-1 line-clamp-3 wrap-break-word rounded-sm border border-(--line) bg-(--bg) p-1.5 font-mono text-[10px] text-(--f2)" />

				<div className="mt-3 flex flex-col gap-2.5">
					<div>
						<div className="mb-1 flex justify-between font-mono text-[10px]">
							<Typography.Label size="xxs" tone="f3" weight="normal">Branch entropy</Typography.Label>
							<span data-f="entropy" />
						</div>
						<div className="font-mono text-[9px] text-(--f4)">
							only gates when the prefix has competing branches · paths <span data-f="paths" />
						</div>
					</div>

					{METERS.map((meter) => (
						<div key={meter.key}>
							<div className="mb-1 flex justify-between font-mono text-[10px]">
								<Typography.Label size="xxs" tone="f3" weight="normal">{meter.label}</Typography.Label>
								<span data-f={meter.key} className="text-(--f1)" />
							</div>
							<div className={meterTrackVariants({ variant: meter.variant, size: "s" })}>
								<div data-meter={meter.key} className="h-full bg-(--meter-tone)" style={{ width: "0%" }} />
							</div>
						</div>
					))}
				</div>

				<div className="mt-3 grid grid-cols-2 gap-1.5 font-mono text-[10px]">
					<div className="flex justify-between">
						<Typography.Label size="xxs" tone="f4" weight="normal">winner</Typography.Label>
						<span data-f="winner" className="text-(--acc)" />
					</div>
					<div className="flex justify-between">
						<Typography.Label size="xxs" tone="f4" weight="normal">candidate</Typography.Label>
						<span data-f="candidate" className="text-(--f1)" />
					</div>
					<div className="flex justify-between">
						<Typography.Label size="xxs" tone="f4" weight="normal">held</Typography.Label>
						<span data-f="held" className="text-(--f1)" />
					</div>
					<div className="flex justify-between">
						<Typography.Label size="xxs" tone="f4" weight="normal">switch</Typography.Label>
						<span className="text-(--f1)">
							<span data-f="switchConf" />
							/
							<span data-f="switchThresh" />
						</span>
					</div>
					<div className="flex justify-between">
						<Typography.Label size="xxs" tone="f4" weight="normal">paths</Typography.Label>
						<span data-f="paths" className="text-(--f1)" />
					</div>
				</div>
			</Panel>
		</div>
	);
};