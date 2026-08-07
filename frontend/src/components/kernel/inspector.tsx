import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	readinessGate,
	sourceHeadlineMetric,
} from "#/components/terminal/kernel-meta";
import { Component } from "#/components/ui/component";
import { cn } from "#/lib/utils";
import { Modal } from "@/components/ui/modal";
import { panelVariants } from "@/components/ui/panel";

/*
KernelInspector shows one kernel's live reading over the focused symbol.

The history line is appended a sample at a time from the same measurement the
list leads with, so the curve and the figure below it are the same reading.
Nothing here is retained by a paint function: the modal mounts when a kernel is
inspected and the frames write into it directly. It previously carried a set of
data-role slots that no wire key was ever routed to, and stayed hidden.
*/
export const KernelInspector = () => {
	const source = useSelector(terminalStore, (state) => state.inspectorSource);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	if (source === null || source === "") {
		return null;
	}

	const copy = kernelCopy(source, "");
	const headline = sourceHeadlineMetric(source);

	return (
		<Modal
			size="m"
			className="inset-y-0 left-70.5 right-83 animate-[symFade_0.18s_ease]"
		>
			<Component registerKey="measurements">
				{({ ref }) => (
					<div
						ref={ref}
						data-scope="source,symbol"
						data-filter={`${source},${focusSymbol}`}
						className="contents"
					>
						<Modal.Header>
							<div className="min-w-0">
								<div className="flex items-center gap-2">
									<span className="font-serif font-semibold text-[19px] text-(--f1) leading-[1.1]">
										{copy.name}
									</span>
									<Component registerKey="readiness">
										{({ ref: gateRef }) => (
											<span ref={gateRef} className="contents">
												<span
													data-paint={readinessGate(source)}
													data-paint-prop="dataset.gate"
													data-paint-class="true:text-(--up) false:text-(--f3)"
													className="group shrink-0 rounded-[3px] border border-(--line2) bg-(--line) px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide text-(--f3)"
												>
													<span className="group-data-[gate=true]:hidden">
														standby
													</span>
													<span className="hidden group-data-[gate=true]:inline">
														live
													</span>
												</span>
											</span>
										)}
									</Component>
								</div>
							</div>
							<Modal.Close aria-label="Close kernel inspector" />
						</Modal.Header>

						<div className="mx-4 mt-3.5 shrink-0">
							<div className="mb-1.5 flex items-center justify-between font-mono text-[9px] text-(--f4) uppercase tracking-widest">
								<span>signal history</span>
								<span data-paint={`${headline}.unit`} />
							</div>
							<svg
								viewBox="0 0 150 30"
								preserveAspectRatio="none"
								className={cn(panelVariants({ size: "bare" }), "block h-13 w-full")}
							>
								<title>Signal history</title>
								{/*
									The line is appended one normalized reading per frame, which
									is what makes it a history rather than a redraw: the samples
									already on it are never recomputed.
								*/}
								<polyline
									data-append={`${headline}.normalized`}
									data-append-limit="150"
									data-append-width="150"
									data-append-height="30"
									fill="none"
									stroke="var(--acc)"
									strokeWidth="1.5"
									vectorEffect="non-scaling-stroke"
								/>
							</svg>
						</div>

						<Modal.Body>
							<div className="grid grid-cols-2 content-start gap-x-3 gap-y-2.5 font-mono text-[10px]">
								{[
									["raw", `${headline}.raw`, ".4f"],
									["normalized", `${headline}.normalized`, ".4f"],
									["maturity", "maturity", ".3f"],
									["confidence", "uncertainty.confidence", ".4f"],
								].map(([label, bind, format]) => (
									<div key={bind} className="flex justify-between gap-2">
										<span className="text-(--f4)">{label}</span>
										<span
											data-paint={bind}
											data-paint-format={format}
											className="text-(--f1)"
										/>
									</div>
								))}
							</div>
							<p className="mt-3 text-[11px] text-(--f3) leading-relaxed">
								{copy.blurb}
							</p>
						</Modal.Body>

						<Modal.Footer>
							<div className="min-w-0 font-mono text-[9.5px] text-(--f4) leading-[1.55]">
								<div data-paint="symbol" />
								<div data-paint="at" data-paint-format="time" />
							</div>
							<button
								type="button"
								onClick={() => terminalStore.actions.selectSource(source)}
								className="shrink-0 cursor-pointer rounded-[3px] border border-[color-mix(in_srgb,var(--acc)_45%,transparent)] bg-[color-mix(in_srgb,var(--acc)_12%,transparent)] px-3 py-2 font-semibold text-[11px] text-(--acc) whitespace-nowrap hover:bg-[color-mix(in_srgb,var(--acc)_22%,transparent)]"
							>
								Open in signal insight →
							</button>
						</Modal.Footer>
					</div>
				)}
			</Component>
		</Modal>
	);
};
