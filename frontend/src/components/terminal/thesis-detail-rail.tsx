import { cn } from "#/lib/utils";
import { Flex } from "@/components/ui/flex";
import { Typography } from "@/components/ui/typography";
import { Panel } from "@/components/ui/panel";
import { causalStore, cognitionStore, positionsStore, strategyStore, useSubscribe } from "#/providers/ws-stores";

const fmt = (value: unknown): string => {
	if (typeof value === "number") return String(value);
	if (typeof value === "string") return value;
	return "—";
};

const Row = ({ label, tone = "text-(--f1)" }: { label: string; tone?: string }) => (
	<Flex.Row align="baseline" justify="between" className="gap-2">
		<Typography.Label size="xxs" tone="f4" weight="normal">{label}</Typography.Label>
		<Typography.Mono size="s" data-row={label} className={cn("min-w-0 truncate text-right", tone)}>—</Typography.Mono>
	</Flex.Row>
);

const Card = ({ title, children }: { title: string; children: React.ReactNode }) => (
	<Panel variant="surface" size="bare" className="px-3 py-2.5">
		<Typography.Label size="lg" tone="f1" className="mb-2 block">{title}</Typography.Label>
		<div className="flex flex-col gap-1">{children}</div>
	</Panel>
);

export const ThesisDetailRail = ({ symbol }: { symbol: string }) => {
	const arbitration = useSubscribe(strategyStore, (state) => {
		const decision = (state?.decisions ?? []).find((entry) => entry.symbol === symbol) ?? null;
		const set = (q: string, value: string) => {
			const el = arbitration.current?.querySelector<HTMLElement>(`[data-row="${q}"]`);
			if (el instanceof HTMLElement) el.textContent = value;
		};

		set("action", decision?.action ?? "—");
		set("thesis score", decision === null ? "—" : decision.thesisScore.toFixed(4));
		set("confidence", decision === null ? "—" : `${(decision.thesisConfidence * 100).toFixed(1)}%`);
		set("cause", decision?.cause ?? "—");
		set("reason", decision?.reason ?? "—");
		set("at", decision === null ? "—" : new Date(decision.at).toISOString().slice(11, 19));
	}, [symbol]);

	const causal = useSubscribe(causalStore, (frames) => {
		const row = (frames ?? []).find((frame) => frame.symbol === symbol) ?? null;
		const set = (q: string, value: string) => {
			const el = causal.current?.querySelector<HTMLElement>(`[data-row="${q}"]`);
			if (el instanceof HTMLElement) el.textContent = value;
		};

		set("association", row?.association === undefined ? "—" : row.association.toFixed(4));
		set("confidence", row?.confidence === undefined ? "—" : row.confidence.toFixed(4));
		set("strength", row?.strength === undefined ? "—" : row.strength.toFixed(4));
	}, [symbol]);

	const cognition = useSubscribe(cognitionStore, (state) => {
		const row = state.cognition[symbol]?.latest() ?? null;
		const set = (q: string, value: string) => {
			const el = cognition.current?.querySelector<HTMLElement>(`[data-row="${q}"]`);
			if (el instanceof HTMLElement) el.textContent = value;
		};

		set("winner", row?.winner === undefined ? "—" : String(row.winner));
		set("contrast", row?.contrast === undefined ? "—" : row.contrast.toFixed(3));
		set("entropy", row?.entropyBits === undefined ? "—" : row.entropyBits.toFixed(3));
		set("paths", row?.lookaheadPaths === undefined ? "—" : String(row.lookaheadPaths));
	}, [symbol]);

	const position = useSubscribe(positionsStore, (state) => {
		const row = state.positions[symbol]?.latest() ?? null;
		const set = (q: string, value: string) => {
			const el = position.current?.querySelector<HTMLElement>(`[data-row="${q}"]`);
			if (el instanceof HTMLElement) el.textContent = value;
		};

		set("status", row?.holding?.status ?? "—");
		set("qty", fmt(row?.holding?.qty));
		set("entry", fmt(row?.holding?.entry_price));
		set("mark", fmt(row?.holding?.mark));
		set("pnl", fmt(row?.holding?.pnl));
		set("stop floor", fmt(row?.holding?.stoploss?.floor));
		set("stop status", row?.holding?.stoploss?.status ?? "—");
	}, [symbol]);

	return (
		<div className="flex min-h-0 flex-col gap-2 overflow-auto pr-1">
			<div ref={arbitration}>
				<Card title="Arbitration">
					<Row label="action" tone="text-(--acc) uppercase" />
					<Row label="thesis score" />
					<Row label="confidence" />
					<Row label="cause" />
					<Row label="reason" />
					<Row label="at" />
				</Card>
			</div>

			<div ref={causal}>
				<Card title="Causal">
					<Row label="association" />
					<Row label="confidence" />
					<Row label="strength" />
				</Card>
			</div>

			<div ref={cognition}>
				<Card title="Cognition">
					<Row label="winner" tone="text-(--acc)" />
					<Row label="contrast" />
					<Row label="entropy" />
					<Row label="paths" />
				</Card>
			</div>

			<div ref={position}>
				<Card title="Position">
					<Row label="status" />
					<Row label="qty" />
					<Row label="entry" />
					<Row label="mark" />
					<Row label="pnl" />
					<Row label="stop floor" />
					<Row label="stop status" />
				</Card>
			</div>
		</div>
	);
};
