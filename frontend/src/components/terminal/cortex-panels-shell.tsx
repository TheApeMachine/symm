import { useEffect, useRef } from "react";
import { signals } from "#/collections/app";
import { Badge } from "#/components/ui/badge";
import { Chip } from "#/components/ui/chip";
import { meterTrackVariants } from "#/components/ui/meter";
import { Panel } from "#/components/ui/panel";
import { Stat } from "#/components/ui/stat";
import { Typography } from "#/components/ui/typography";

export const CortexPanelsShell = ({ symbol }: { symbol: string }) => {
	const root = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const apply = () => {
			if (!root.current) return;
			const targetRow = signals.cognition.state[symbol]?.getLast() ?? null;

			const set = (q: string, value: string) => {
				const el = root.current?.querySelector<HTMLElement>(`[data-f="${q}"]`);
				if (el) el.textContent = value;
			};

			const metricMap = new Map<string, number>();
			if (targetRow) {
				for (const m of targetRow.metrics) {
					const name = typeof m.name === "string" ? m.name : "";
					if (name) {
						metricMap.set(name, m.hasNormalized ? m.normalized : m.raw);
					}
				}
			}

			const contrastVal = metricMap.get("contrast");
			const surprisalVal = metricMap.get("surprisal");
			const stabilityVal = metricMap.get("stability");
			const ambiguityVal = metricMap.get("ambiguity");

			set("winner", targetRow ? symbol : "—");
			set(
				"confidence",
				stabilityVal !== undefined ? `${(stabilityVal * 100).toFixed(1)}%` : "—",
			);
			set("contrast", contrastVal !== undefined ? contrastVal.toFixed(3) : "—");
			set("entropy", surprisalVal !== undefined ? surprisalVal.toFixed(3) : "—");
			set(
				"ambiguous",
				ambiguityVal !== undefined
					? ambiguityVal > 0.5
						? "true"
						: "false"
					: "—",
			);
			set("remFrom", "—");
			set("remThrough", "—");
			set("remReplays", "—");

			const replays = root.current.querySelector<HTMLElement>("[data-replays]");
			if (replays) {
				replays.textContent = "—";
			}

			const basin = root.current.querySelector<HTMLElement>("[data-basin]");
			if (basin instanceof HTMLElement) {
				basin.style.width =
					typeof stabilityVal === "number"
						? `${Math.min(100, Math.max(0, stabilityVal * 100)).toFixed(1)}%`
						: "0%";
			}

			const entropy = root.current.querySelector<HTMLElement>("[data-entropy]");
			if (entropy instanceof HTMLElement) {
				entropy.style.width =
					typeof surprisalVal === "number"
						? `${Math.min(100, Math.max(0, surprisalVal * 100)).toFixed(1)}%`
						: "0%";
			}
		};

		apply();
		const subscription = signals.cognition.subscribe(apply);
		return () => subscription.unsubscribe();
	}, [symbol]);

	return (
		<div ref={root} className="flex flex-col gap-3.5">
			<Panel>
				<Panel.Header
					title="Attractor basin · classify"
					meta={<Badge label="classify" variant="warning" />}
				/>
				<Panel.Caption>softmax posterior · b/[class]/[sequence]</Panel.Caption>
				<div className="flex flex-col gap-2">
					<div className="flex items-center justify-between font-mono text-[10px]">
						<Typography.Span data-f="winner" className="text-(--f3)" />
						<Typography.Span data-f="confidence" className="text-(--f1)" />
					</div>
					<div className={meterTrackVariants({ variant: "info", size: "m" })}>
						<div
							data-basin
							className="h-full bg-(--meter-tone)"
							style={{ width: "0%" }}
						/>
					</div>
				</div>
			</Panel>

			<Panel>
				<Panel.Header title="Contrastive evidence" />
				<Panel.Caption>routing margin · winner vs runner-up</Panel.Caption>
				<div className="grid grid-cols-2 gap-2.5 text-center">
					<Stat
						layout="feature"
						label="contrast"
						value={<Typography.Span data-f="contrast" />}
						variant="warning"
					/>
					<Stat
						layout="feature"
						label="entropy bits"
						value={<Typography.Span data-f="entropy" />}
						variant="warning"
					/>
				</div>
			</Panel>

			<Panel>
				<Panel.Header
					title="Branch entropy gate"
					meta={<Typography.Label size="xs" tone="f3" data-f="ambiguous" />}
				/>
				<Panel.Caption>shannon H vs uniform threshold</Panel.Caption>
				<div>
					<div
						className={meterTrackVariants({ variant: "success", size: "m" })}
					>
						<div
							data-entropy
							className="h-full bg-(--meter-tone)"
							style={{ width: "0%" }}
						/>
					</div>
				</div>
			</Panel>

			<Panel>
				<Panel.Header
					title="REM consolidation"
					meta={
						<Chip data-consolidating label={<Typography.Span data-replays />} />
					}
				/>
				<Panel.Caption>
					episodic replay · decay · retroactive inhibition
				</Panel.Caption>
				<div className="grid grid-cols-3 gap-2">
					<Stat
						layout="feature"
						label="from"
						value={<Typography.Span data-f="remFrom" />}
					/>
					<Stat
						layout="feature"
						label="replays"
						value={<Typography.Span data-f="remReplays" />}
					/>
					<Stat
						layout="feature"
						label="through"
						value={<Typography.Span data-f="remThrough" />}
					/>
				</div>
			</Panel>
		</div>
	);
};
