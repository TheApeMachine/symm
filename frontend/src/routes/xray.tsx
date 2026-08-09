import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useLayoutEffect, useRef } from "react";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { paintXrayHierarchy } from "#/components/terminal/xray-hierarchy";
import { paintXrayLatent } from "#/components/terminal/xray-latent";
import {
	XrayFactsPanel,
	XrayHawkesPanel,
	XrayHierarchyPanel,
	XrayLatentPanel,
	XrayManifoldPanel,
} from "#/components/terminal/xray-panels";
import { Component } from "#/components/ui/component";
import type { JSONSerializable } from "#/components/ui/paint";
import { getLastFrame, registerPainter } from "#/providers/ws-stores";

const XrayPaintBridge = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const latestResonance = useRef<JSONSerializable | null>(null);

	useLayoutEffect(() => {
		const paint = (updates: JSONSerializable) => {
			latestResonance.current = updates;
			paintXrayHierarchy(updates, focusSymbol);
			paintXrayLatent(updates, focusSymbol);
		};

		const unregister = registerPainter("resonance", paint);

		/*
			A fresh mount has no `latestResonance` even when the feed already has
			data — routing tears the previous instance down along with its ref.
			Replaying the retained last frame is what makes revisiting this page
			show data immediately instead of sitting blank until the next tick.
		*/
		const seed = latestResonance.current ?? getLastFrame("resonance");

		if (seed !== null && seed !== undefined) {
			paint(seed);
		}

		return unregister;
	}, [focusSymbol]);

	return null;
};

/*
XrayCarrierBar lists the symbols the predictive stack actually holds a carrier
for. The resonance batch is the list — one slot per retained carrier — so the
bar can never offer a symbol this surface has nothing to show for.
*/
const XrayCarrierBar = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);

	return (
		<Component registerKey="resonance">
			{({ ref, slots }) => (
				<div
					ref={ref}
					className="flex h-11.5 shrink-0 items-center gap-2 overflow-x-auto border-(--line) border-b bg-(--surface) px-3.5"
				>
					<span className="mr-1 shrink-0 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Inspect symbol
					</span>
					{slots.length === 0 ? (
						<span className="font-mono text-[10px] text-(--f4)">
							waiting for resonance carriers
						</span>
					) : null}
					{slots.map((index) => (
						<button
							key={index}
							type="button"
							data-index={index}
							data-paint="symbol"
							data-paint-class={`${focusSymbol}:border-(--acc),bg-(--raised),text-(--acc)`}
							onClick={(event) => {
								const symbol = event.currentTarget.textContent?.trim() ?? "";

								if (symbol === "") {
									return;
								}

								appStore.actions.updateFocusSymbol(symbol);
								terminalStore.actions.selectFocusSymbol(symbol);
							}}
							className="shrink-0 cursor-pointer rounded-[3px] border border-(--line2) bg-transparent px-2.75 py-1 font-medium font-mono text-[11px] text-(--f3) hover:border-(--acc)"
						/>
					))}
				</div>
			)}
		</Component>
	);
};

/*
Xray composes store-painted panels. Predictive coding reads resonance.layers
only; manifold / ρ stay on their own side panel and never feed the hierarchy.
*/
const RouteComponent = () => (
	<div className="flex h-full min-w-275 flex-col">
		<XrayPaintBridge />
		<XrayCarrierBar />
		<div className="grid min-h-0 flex-1 grid-cols-[minmax(520px,1fr)_352px]">
			<div className="flex min-h-0 flex-col overflow-auto border-(--line) border-r">
				<XrayHierarchyPanel />
				<XrayHawkesPanel />
			</div>

			<div className="flex min-h-0 flex-col overflow-auto bg-(--surface)">
				<div className="shrink-0 px-3.5 pt-3 pb-1.5">
					<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Latent manifold
					</div>
					<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
						universe embedding · clustered by regime · focus pulses
					</div>
				</div>
				<div className="relative mx-2 h-75 shrink-0">
					<XrayLatentPanel />
					<div className="pointer-events-none absolute bottom-1.5 left-2.5 font-mono text-[8.5px] text-(--f4)">
						latent-1 →
					</div>
					<div className="pointer-events-none absolute top-2.5 left-1.5 font-mono text-[8.5px] text-(--f4) [writing-mode:vertical-rl]">
						latent-2 →
					</div>
				</div>

				<XrayFactsPanel />
				<XrayManifoldPanel />
			</div>
		</div>
	</div>
);

export const Route = createFileRoute("/xray")({
	component: RouteComponent,
});
