import { Flex } from "#/components/ui/flex";
import { cn } from "#/lib/utils";
import { measurementsStore, positionsStore, strategyStore, tickStore, useSubscribe } from "#/providers/ws-stores";

const wrap = (root: React.RefObject<HTMLDivElement | null>, which: string, value: string | number) => {
	const el = root.current?.querySelector<HTMLElement>(`[data-read=${which}]`);

	if (el instanceof HTMLElement) {
		el.textContent = String(value);
	}
};

const Reading = ({ label, which, accent = false }: { label?: string; which: string; accent?: boolean }) => (
	<Flex.Row align="center" gap={1}>
		{label ? <span>{label}</span> : null}
		<span
			data-read={which}
			className={cn(accent ? "text-(--acc)" : "font-semibold text-(--f1)", "tabular-nums")}
		>
			—
		</span>
	</Flex.Row>
);

export const Pulse = () => {
	const tickRoot = useSubscribe(tickStore, (state) => {
		wrap(tickRoot, "tick", state?.count ?? "—");
	});
	const stratRoot = useSubscribe(strategyStore, (state) => {
		wrap(stratRoot, "phase", state?.outcome ?? "—");
		wrap(stratRoot, "cand", state === null ? "—" : state.decisions.length);
	});
	const measRoot = useSubscribe(measurementsStore, (state) => {
		wrap(measRoot, "meas", Object.keys(state.measurements).length);
	});
	const posRoot = useSubscribe(positionsStore, (state) => {
		wrap(posRoot, "open", Object.keys(state.positions).length);
	});

	return (
		<Flex.Row
			align="center"
			gap={4}
			className="h-8 shrink-0 border-(--line) border-b bg-(--sunken) px-3.5 font-mono text-[11px] text-(--f3)"
		>
			<div ref={tickRoot} className="contents">
				<Reading which="tick" />
				<div ref={stratRoot} className="contents">
					<Reading label="phase" which="phase" accent />
					<Reading label="cand" which="cand" />
				</div>
				<div ref={measRoot} className="contents">
					<Reading label="meas" which="meas" />
				</div>
				<div ref={posRoot} className="contents">
					<Reading label="open" which="open" />
				</div>
			</div>
		</Flex.Row>
	);
};
