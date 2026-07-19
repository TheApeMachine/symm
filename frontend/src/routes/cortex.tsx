import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useEffect, useMemo, useRef, useState } from "react";
import { appStore } from "#/collections/app";
import type { CognitiveReading, Instrument } from "#/collections/types";
import { resizeCanvas } from "#/components/terminal/canvas";
import { drawCognitiveTree } from "#/components/terminal/cognitive-viz";
import { CortexLeafRoster } from "#/components/terminal/cortex-draw";
import {
	CortexBeamList,
	CortexSidePanels,
} from "#/components/terminal/cortex-panels";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { getWorker } from "#/providers/websocket";

const isConcreteScope = (scope: string): boolean =>
	scope !== "" && scope !== "stream";

/*
cognitiveScopes lists stable lexical scopes from a symbol-keyed reading map.
*/
export const cognitiveScopes = (
	readings: Record<string, CognitiveReading>,
): string[] => Object.keys(readings).sort();

const CortexCanvas = ({ reading }: { reading: CognitiveReading | null }) => {
	const canvasRef = useRef<HTMLCanvasElement | null>(null);
	const readingRef = useRef(reading);
	const rosterRef = useRef(new CortexLeafRoster());
	readingRef.current = reading;

	useEffect(() => {
		const canvas = canvasRef.current;

		if (canvas === null) {
			return;
		}

		let frame = 0;

		const render = () => {
			const context = resizeCanvas(canvas);

			if (context === null) {
				return;
			}

			drawCognitiveTree(
				context,
				canvas.clientWidth,
				canvas.clientHeight,
				readingRef.current as Record<string, unknown> | null,
				rosterRef.current,
			);
		};

		const tick = () => {
			render();
			frame = requestAnimationFrame(tick);
		};

		frame = requestAnimationFrame(tick);
		const observer = new ResizeObserver(render);
		observer.observe(canvas);

		return () => {
			cancelAnimationFrame(frame);
			observer.disconnect();
		};
	}, []);

	return (
		<canvas
			ref={canvasRef}
			className="absolute inset-0 block h-full w-full bg-(--bg)"
		/>
	);
};

export const activeScopeFor = (
	available: Set<string>,
	focusSymbol: string,
	scopes: string[],
): string | null => {
	if (isConcreteScope(focusSymbol) && available.has(focusSymbol)) {
		return focusSymbol;
	}

	return scopes[0] ?? null;
};

const RouteComponent = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const online = useSelector(appStore, (state) => state.online);
	const { updateFocusSymbol } = appStore.actions;
	const [cognitive, setCognitive] = useState<
		Record<string, CognitiveReading>
	>({});
	const [instrumentSymbols, setInstrumentSymbols] = useState<string[]>([]);

	useDirectStorePaint(
		getWorker(),
		[
			{ store: "cognitive", key: "" },
			{ store: "instruments", key: "" },
		],
		(buffers) => {
			const readings = (buffers["cognitive:"] ?? []) as CognitiveReading[];
			const bySymbol: Record<string, CognitiveReading> = {};

			for (const reading of readings) {
				const key = reading.symbol || reading.scope || "";

				if (key !== "") {
					bySymbol[key] = reading;
				}
			}

			setCognitive(bySymbol);
			setInstrumentSymbols(
				((buffers["instruments:"] ?? []) as Instrument[])
					.map((instrument) => instrument.symbol)
					.sort(),
			);
		},
		[online],
	);

	const scopes = useMemo(() => {
		const readingScopes = cognitiveScopes(cognitive);
		const available = new Set(readingScopes);
		const instrumentScopes = instrumentSymbols.filter((symbol) =>
			available.has(symbol),
		);
		const instrumentScopeSet = new Set(instrumentScopes);

		return [
			...instrumentScopes,
			...readingScopes.filter((scope) => !instrumentScopeSet.has(scope)),
		];
	}, [instrumentSymbols, cognitive]);
	const activeScope = activeScopeFor(new Set(scopes), focusSymbol, scopes);
	const reading = activeScope === null ? null : (cognitive[activeScope] ?? null);
	const treeMeta =
		reading === null
			? ""
			: `${reading.nodeCount ?? 0} nodes · depth ${reading.maxHops ?? 0} · ${reading.scope}`;

	return (
		<div className="flex h-full min-w-[1140px] flex-col">
			<div className="flex h-[46px] shrink-0 items-center gap-2 overflow-x-auto border-(--line) border-b bg-(--surface) px-3.5">
				<span className="mr-1 shrink-0 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
					Sensory context
				</span>
				{scopes.map((scope) => {
					const active = scope === activeScope;

					return (
						<button
							key={scope}
							type="button"
							data-symbol={scope}
							className="shrink-0 rounded-[3px] border px-[11px] py-1 font-mono font-medium text-[11px]"
							style={{
								borderColor: active ? "var(--acc)" : "var(--line)",
								background: active
									? "color-mix(in srgb,var(--acc) 14%,transparent)"
									: "var(--surface)",
								color: active ? "var(--acc)" : "var(--f3)",
							}}
							onClick={() => updateFocusSymbol(scope)}
						>
							{scope}
						</button>
					);
				})}
				<span className="ml-auto shrink-0 font-mono text-[10px] text-(--f4)">
					{treeMeta}
				</span>
			</div>
			<div className="grid min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_364px]">
				<div className="flex min-h-0 flex-col border-(--line) border-r">
					<div className="relative min-h-0 flex-[1.55] overflow-hidden bg-(--sunken)">
						<CortexCanvas key={activeScope ?? "none"} reading={reading} />
						<div className="pointer-events-none absolute top-3 left-3.5">
							<div className="font-semibold text-[10px] text-(--f2) uppercase tracking-[0.13em]">
								Sensory prefix tree · s/[sequence]
							</div>
							<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
								edge = P(next token | prefix) · amber = MAP beam path
							</div>
						</div>
						<div className="pointer-events-none absolute top-3 right-3.5 flex gap-[13px] font-mono text-[9px] text-(--f3)">
							<span className="inline-flex items-center gap-[5px]">
								<span className="h-[2px] w-2.5 bg-(--acc)" />
								beam
							</span>
							<span className="inline-flex items-center gap-[5px]">
								<span className="h-[2px] w-2.5 bg-(--line2)" />
								branch
							</span>
						</div>
					</div>

					<div className="flex min-h-0 flex-1 flex-col border-(--line) border-t bg-(--surface)">
						<div className="flex shrink-0 items-center justify-between border-(--line) border-b px-3 py-2">
							<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
								Beam search lookahead
							</span>
							<span className="font-mono text-[9.5px] text-(--f4)">
								width {reading?.beamWidth ?? 0} · {reading?.maxHops ?? 0} hops ·
								log-prob
							</span>
						</div>
						<CortexBeamList
							reading={reading as Record<string, unknown> | null}
						/>
					</div>
				</div>

				<div className="min-h-0 overflow-auto bg-(--surface) p-3.5">
					<CortexSidePanels
						reading={reading as Record<string, unknown> | null}
					/>
				</div>
			</div>
		</div>
	);
};

export const Route = createFileRoute("/cortex")({
	component: RouteComponent,
});
