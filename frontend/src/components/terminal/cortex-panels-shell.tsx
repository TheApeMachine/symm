import { Badge } from "@/components/ui/badge";
import { meterTrackVariants } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import { Stat } from "@/components/ui/stat";
import { Typography } from "@/components/ui/typography";
import { cognitionStore, useSubscribe } from "#/providers/ws-stores";

export const CortexPanelsShell = ({ symbol }: { symbol: string }) => {
	const root = useSubscribe(cognitionStore, (state) => {
		const row = state.cognition[symbol]?.latest() ?? null;

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f="${q}"]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		set("winner", String(row?.winner ?? "—"));
		set("confidence", row?.confidence === undefined ? "—" : `${(row.confidence * 100).toFixed(1)}%`);
		set("contrast", row?.contrast === undefined ? "—" : row.contrast.toFixed(3));
		set("entropy", row?.entropyBits === undefined ? "—" : row.entropyBits.toFixed(3));
		set("ambiguous", row?.ambiguous === undefined ? "—" : String(row.ambiguous));
		set("remFrom", row?.remFrom === undefined ? "—" : String(row.remFrom));
		set("remThrough", row?.remThrough === undefined ? "—" : String(row.remThrough));
		set("remReplays", String(row?.remReplays ?? "—"));

		const replays = root.current?.querySelector<HTMLElement>("[data-replays]");

		if (replays instanceof HTMLElement) {
			replays.textContent = String(row?.remReplays ?? "—");
		}

		const basin = root.current?.querySelector<HTMLElement>("[data-basin]");

		if (basin instanceof HTMLElement) {
			const value = row?.confidence;
			basin.style.width =
				typeof value === "number"
					? `${Math.min(100, Math.max(0, value * 100)).toFixed(1)}%`
					: "0%";
		}

		const entropy = root.current?.querySelector<HTMLElement>("[data-entropy]");

		if (entropy instanceof HTMLElement) {
			const value = row?.entropyBits;
			entropy.style.width =
				typeof value === "number"
					? `${Math.min(100, Math.max(0, value * 100)).toFixed(1)}%`
					: "0%";
		}

	}, [symbol]);

	return (
		<div ref={root} className="flex flex-col gap-3.5">
			<Panel>
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">Attractor basin · classify</span>
					<Badge label="classify" variant="warning" />
				</div>
				<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">softmax posterior · b/[class]/[sequence]</div>
				<div className="flex flex-col gap-2">
					<div className="flex items-center justify-between font-mono text-[10px]">
						<Typography.Span data-f="winner" className="text-(--f3)" />
						<Typography.Span data-f="confidence" className="text-(--f1)" />
					</div>
					<div className={meterTrackVariants({ variant: "info", size: "m" })}>
						<div data-basin className="h-full bg-(--meter-tone)" style={{ width: "0%" }} />
					</div>
				</div>
			</Panel>

			<Panel>
				<div className="font-semibold text-[12px] text-(--f1)">Contrastive evidence</div>
				<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">routing margin · winner vs runner-up</div>
				<div className="grid grid-cols-2 gap-2.5 text-center">
					<Stat layout="feature" label="contrast" value={<Typography.Span data-f="contrast" />} variant="warning" />
					<Stat layout="feature" label="entropy bits" value={<Typography.Span data-f="entropy" />} variant="warning" />
				</div>
			</Panel>

			<Panel>
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">Branch entropy gate</span>
					<Typography.Span data-f="ambiguous" className="font-semibold text-[9px] uppercase tracking-wide" />
				</div>
				<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">shannon H vs uniform threshold</div>
				<div>
					<div className={meterTrackVariants({ variant: "success", size: "m" })}>
						<div data-entropy className="h-full bg-(--meter-tone)" style={{ width: "0%" }} />
					</div>
				</div>
			</Panel>

			<Panel>
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">REM consolidation</span>
					<span data-consolidating className="group rounded-full border border-(--line2) px-2.25 py-px font-mono text-[9px]">
						<span data-replays className="text-(--f3)" />
					</span>
				</div>
				<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">episodic replay · decay · retroactive inhibition</div>
				<div className="grid grid-cols-3 gap-2">
					<Stat layout="feature" label="from" value={<Typography.Span data-f="remFrom" />} />
					<Stat layout="feature" label="replays" value={<Typography.Span data-f="remReplays" />} />
					<Stat layout="feature" label="through" value={<Typography.Span data-f="remThrough" />} />
				</div>
			</Panel>
		</div>
	);
};