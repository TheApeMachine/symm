import { useSelector } from "@tanstack/react-store";
import { atoms, signals } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { sourceHeadline } from "#/components/terminal/kernel-meta";
import {
	Badge,
	Button,
	Flex,
	Meter,
	Sparkline,
	Typography,
} from "#/components/ui";
import { asFinite } from "#/lib/format";
import { kernelRowAt } from "#/lib/kernel-row";
import { cn } from "#/lib/utils";

export const Metric = ({
	name,
	compact = false,
}: {
	name: string;
	compact?: boolean;
}) => {
	const focus = useSelector(atoms.focus, (state) => state);
	const headline = sourceHeadline(name);

	const sample = useSelector(signals[name], (state) => {
		if (!state || !headline) return null;

		const row = kernelRowAt(state, focus);
		if (!row || typeof row !== "object") return null;

		return asFinite((row as Record<string, unknown>)[headline]);
	});

	const live = sample !== null;
	const percent = live ? Math.max(0, Math.min(100, Math.abs(sample) * 100)) : 0;

	return (
		<div className="min-h-0 flex-1 flex flex-col overflow-hidden">
			<Button
				variant="bare"
				shape="block"
				className="min-h-0 justify-start border-(--line) border-b"
				onClick={() => {
					atoms.focusMetric.set(() => name);
					terminalStore.actions.inspectSource(name);
				}}
				fullHeight
			>
				<Flex.Row
					align="center"
					justify="between"
					gap={2}
					padding={1}
					fullWidth
					className="shrink-0"
				>
					<Typography.Span
						variant="f1"
						semibold
						truncate
						className={cn(compact && "text-[10px]")}
					>
						{name.toUpperCase()}
					</Typography.Span>
					<Badge
						label={live ? "Live" : "Standby"}
						variant={live ? "info" : "disabled"}
						size="xxs"
					/>
				</Flex.Row>
				<Sparkline
					points={live ? [sample] : []}
					active={live}
					title={`${name} sparkline`}
					className="mt-0 min-h-0 flex-1"
				/>
				<Flex.Row
					align="center"
					gap={2}
					padding={1}
					fullWidth
					className="shrink-0"
				>
					<Meter
						layout="bar"
						size="xxs"
						percent={percent}
						variant="warning"
						className="flex-1"
						title={headline ? `${name}: ${headline}` : name}
					/>
					<Typography.Mono
						size="xxs"
						tone="f2"
						className="w-11 shrink-0 text-right font-mono"
					>
						{live ? sample.toFixed(3) : "--"}
					</Typography.Mono>
				</Flex.Row>
			</Button>
		</div>
	);
};
