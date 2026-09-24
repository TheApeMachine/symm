import { useSelector } from "@tanstack/react-store";
import { focusAtom, signals } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { usePaintStore } from "#/components/ui/paint";
import { Rail } from "#/components/ui/rail";
import { Typography } from "#/components/ui/typography";
import { CortexBeamShell } from "./cortex-beam-shell";
import { CortexCanvas } from "./cortex-canvas";
import { CortexPanelsShell } from "./cortex-panels-shell";

export const CortexSurface = () => {
	const focusSymbol = useSelector(focusAtom, (state) => state);

	const rootRef = usePaintStore(
		signals.cognition,
		(state) => {
			const targetRow = state[focusSymbol]?.getLast() ?? null;
			return {
				fields: {
					winner: targetRow?.winner() ?? "—",
					sequence: targetRow?.sequence() ?? "—",
				},
			};
		},
		[focusSymbol],
	);

	return (
		<Flex.Column ref={rootRef} fullHeight className="min-w-285">
			<Rail.Header
				title="Sensory context"
				meta={
					<>
						<Typography.Span data-f="winner" data-winner /> ·{" "}
						<Typography.Span data-f="sequence" data-sequence />
					</>
				}
				className="h-11.5 bg-(--surface) px-3.5"
			/>
			<Grid
				cols={2}
				responsive={false}
				className="min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_364px]"
			>
				<Flex.Column className="min-h-0 border-(--line) border-r">
					<div className="relative min-h-0 flex-[1.55] overflow-hidden bg-(--sunken)">
						<CortexCanvas
							symbol={focusSymbol}
							className="absolute inset-0 block h-full w-full bg-(--bg)"
						/>
						<div className="pointer-events-none absolute top-3 left-3.5">
							<Typography.Label size="xs" tone="f2">
								Sensory prefix tree · s/[sequence]
							</Typography.Label>
							<Typography.Mono size="xs" tone="f4" className="mt-0.5">
								edge = P(next token | prefix) · amber = MAP beam path
							</Typography.Mono>
						</div>
						<Flex.Row
							gap={3}
							className="pointer-events-none absolute top-3 right-3.5 font-mono text-[9px] text-(--f3)"
						>
							<span className="inline-flex items-center gap-1.25">
								<span className="h-0.5 w-2.5 bg-(--acc)" />
								beam
							</span>
							<span className="inline-flex items-center gap-1.25">
								<span className="h-0.5 w-2.5 bg-(--line2)" />
								branch
							</span>
						</Flex.Row>
					</div>
					<Flex.Column className="min-h-0 flex-1 border-(--line) border-t bg-(--surface)">
						<Rail.Header
							title="Beam search lookahead"
							meta="log-prob"
							className="px-3 py-2"
						/>
						<CortexBeamShell symbol={focusSymbol} />
					</Flex.Column>
				</Flex.Column>
				<Rail
					position="none"
					surface="surface"
					width="xwide"
					className="min-h-0 overflow-auto p-3.5"
				>
					<CortexPanelsShell symbol={focusSymbol} />
				</Rail>
			</Grid>
		</Flex.Column>
	);
};
