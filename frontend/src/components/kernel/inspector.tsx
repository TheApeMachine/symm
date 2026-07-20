import { terminalStore } from "#/collections/terminal";
import {
	bindInspector,
	closeInspectorShell,
} from "#/components/terminal/kernel-list";
import { cn } from "#/lib/utils";
import { Modal } from "@/components/ui/modal";
import { panelVariants } from "@/components/ui/panel";
import { Spinner } from "@/components/ui/spinner";

/*
KernelInspector is a persistent modal shell. It mounts once; open/close only
toggles visibility while paint updates title, meters, and spark views.
*/
export const KernelInspector = () => (
	<Modal
		ref={bindInspector}
		hidden
		onClose={closeInspectorShell}
		size="m"
		className="inset-y-0 left-[282px] right-[332px] animate-[symFade_0.18s_ease]"
	>
		<Modal.Header>
			<div className="min-w-0">
				<div className="flex items-center gap-2">
					<span
						data-role="title"
						className="font-serif font-semibold text-[19px] text-(--f1) leading-[1.1]"
					/>
					<span
						data-role="status"
						className="shrink-0 rounded-[3px] border border-(--line2) bg-(--line) px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide text-(--f3)"
					>
						Standby
					</span>
				</div>
			</div>
			<Modal.Close
				aria-label="Close kernel inspector"
				onClick={closeInspectorShell}
			/>
		</Modal.Header>
		<div className="mx-4 mt-3.5 shrink-0">
			<div className="mb-1.5 flex items-center justify-between font-mono text-[9px] text-(--f4) uppercase tracking-widest">
				<span>signal history</span>
				<span data-role="series" />
			</div>
			<svg
				viewBox="0 0 150 30"
				preserveAspectRatio="none"
				className={cn(
					panelVariants({ size: "bare" }),
					"block h-[52px] w-full",
				)}
			>
				<title>Signal history</title>
				<polyline
					data-role="spark-line"
					fill="none"
					stroke="var(--acc)"
					strokeWidth="1.5"
					vectorEffect="non-scaling-stroke"
				/>
			</svg>
		</div>
		<Modal.Body
			data-role="metrics"
			className="grid grid-cols-2 content-start gap-x-3 gap-y-2.5"
		>
			<div
				data-role="metrics-waiting"
				className="col-span-2 flex min-h-[88px] items-center justify-center"
			>
				<Spinner size="m" label="Sampling meters" />
			</div>
		</Modal.Body>
		<Modal.Footer>
			<div className="min-w-0 font-mono text-[9.5px] text-(--f4) leading-[1.55]">
				<div data-role="symbol" />
				<div data-role="observed" />
			</div>
			<button
				type="button"
				onClick={() => {
					const source = terminalStore.state.inspectorSource;

					if (source === null) {
						return;
					}

					terminalStore.actions.selectSource(source);
					closeInspectorShell();
				}}
				className="shrink-0 cursor-pointer rounded-[3px] border border-[color-mix(in_srgb,var(--acc)_45%,transparent)] bg-[color-mix(in_srgb,var(--acc)_12%,transparent)] px-3 py-2 font-semibold text-[11px] text-(--acc) whitespace-nowrap hover:bg-[color-mix(in_srgb,var(--acc)_22%,transparent)]"
			>
				Open in signal insight →
			</button>
		</Modal.Footer>
	</Modal>
);
