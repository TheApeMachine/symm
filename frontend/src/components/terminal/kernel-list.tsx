import { useSelector } from "@tanstack/react-store";
import { appStore, DEFAULT_KERNELS } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { cn } from "#/lib/utils";
import { measurementsStore, useSubscribe } from "#/providers/ws-stores";

const interactive = (compact: boolean, source: string) => {
	if (compact) {
		return () => {
			terminalStore.actions.selectSource(source);
		};
	}

	return () => {
		terminalStore.actions.inspectSource(source);
	};
};

export const KernelList = ({
	compact = false,
	sources = DEFAULT_KERNELS,
}: {
	compact?: boolean;
	sources?: string[];
}) => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	const root = useSubscribe(measurementsStore, (state) => {
		for (const source of sources) {
			const row = state.measurements[`${source}\u0000${focusSymbol}`]?.latest();
			const cell = root.current?.querySelector<HTMLElement>(`[data-kernel="${source}"]`);

			if (cell === null || cell === undefined) {
				continue;
			}

			const snr = row?.metrics?.snr?.raw;

			const set = (q: string, value: string) => {
				const el = cell.querySelector<HTMLElement>(`[data-k="${q}"]`);

				if (el instanceof HTMLElement) {
					el.textContent = value;
				}
			};

			set("snr1", snr === undefined ? "" : snr.toFixed(1));
			set("symbol", row?.symbol ?? "");
		}
	}, [focusSymbol, sources]);

	return (
		<div ref={root} className="min-h-0 overflow-auto">
			{sources.map((source) => (
				<button
					key={source}
					type="button"
					data-kernel={source}
					onClick={interactive(compact, source)}
					className="block w-full cursor-pointer border-(--line) border-b border-l-2 border-l-transparent bg-transparent px-3 py-2.5 text-left font-[inherit] hover:bg-(--raised)"
				>
					<div className="flex items-center justify-between gap-2">
						<span className={cn("truncate font-semibold text-(--f1)", compact ? "text-xs" : "text-[12.5px]")}>
							{source}
						</span>
						<span data-k="symbol" className="font-mono text-[9.5px] text-(--f4)" />
					</div>
					<div className="mt-0.5 flex items-baseline gap-1.5 truncate font-mono text-[9.5px] text-(--f4)">
						<span data-k="snr1" className="text-(--acc)" />
					</div>
				</button>
			))}
		</div>
	);
};
