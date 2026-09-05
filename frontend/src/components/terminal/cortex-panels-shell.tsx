import { useEffect, useRef } from "react";
import { cognitionStore } from "#/collections/app";
import { Badge } from "#/components/ui/badge";
import { Chip } from "#/components/ui/chip";
import { meterTrackVariants } from "#/components/ui/meter";
import { Panel } from "#/components/ui/panel";
import { Stat } from "#/components/ui/stat";
import { Typography } from "#/components/ui/typography";

export const CortexPanelsShell = ({ symbol }: { symbol: string }) => {
	const root = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const apply = (state: typeof cognitionStore.state) => {
			if (!root.current) return;
			const targetRow = state.getLast(symbol) ?? null;

			const set = (q: string, value: string) => {
				const el = root.current?.querySelector<HTMLElement>(`[data-f="${q}"]`);
				if (el) el.textContent = value;
			};

			set("winner", targetRow?.winner() ?? "—");
			set(
				"confidence",
				targetRow ? `${(targetRow.confidence() * 100).toFixed(1)}%` : "—",
			);
			set("contrast", targetRow ? targetRow.contrast().toFixed(3) : "—");
			set("entropy", targetRow ? targetRow.entropyBits().toFixed(3) : "—");
			set("ambiguous", targetRow ? String(targetRow.ambiguous()) : "—");
			set("remFrom", targetRow ? String(targetRow.remFromNs()) : "—");
			set("remThrough", targetRow ? String(targetRow.remThroughNs()) : "—");
			set("remReplays", targetRow ? String(targetRow.remReplays()) : "—");

			const replays = root.current.querySelector<HTMLElement>("[data-replays]");
			if (replays) {
				replays.textContent = targetRow ? String(targetRow.remReplays()) : "—";
			}

			const basin = root.current.querySelector<HTMLElement>("[data-basin]");
			if (basin instanceof HTMLElement) {
				const value = targetRow?.confidence();
				basin.style.width =
					typeof value === "number"
						? `${Math.min(100, Math.max(0, value * 100)).toFixed(1)}%`
						: "0%";
			}

			const entropy = root.current.querySelector<HTMLElement>("[data-entropy]");
			if (entropy instanceof HTMLElement) {
				const value = targetRow?.entropyBits();
				entropy.style.width =
					typeof value === "number"
						? `${Math.min(100, Math.max(0, value * 100)).toFixed(1)}%`
						: "0%";
			}
		};

		apply(cognitionStore.state);
		const subscription = cognitionStore.subscribe(apply);
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
