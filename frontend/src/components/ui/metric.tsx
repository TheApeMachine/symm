import { focusMetricAtom } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	Badge,
	Button,
	Component,
	Flex,
	Meter,
	Sparkline,
	Typography,
} from "#/components/ui";
import { cn } from "#/lib/utils";

interface MetricProps {
	name: string;
	compact?: boolean;
}

export const Metric = ({ name, compact = false }: MetricProps) => {
	return (
		<Component>
			<Button
				variant="bare"
				shape="block"
				className="border-(--line) border-b"
				onClick={() => {
					focusMetricAtom.set(() => name);
					terminalStore.actions.inspectSource(name);
				}}
				fullHeight
			>
				<Flex.Row
					align="center"
					justify="between"
					gap={2}
					padding={2}
					fullWidth
				>
					<Typography.Span
						variant="f1"
						semibold
						truncate
						className={cn(compact && "text-[10px]")}
					>
						{name.toUpperCase()}
					</Typography.Span>
					<Badge data-k="badge" label="Standby" variant="disabled" size="xxs" />
				</Flex.Row>
				<Sparkline data-k="sparkline" title={`${name} sparkline`} />
				<Flex.Row align="center" gap={2} padding={2} fullWidth>
					<Meter
						data-metric="confidence"
						data-k="bar"
						layout="bar"
						size="xxs"
						percent={0}
						variant="warning"
						className="flex-1"
						title={`${name} confidence`}
					/>
					<Typography.Mono
						data-metric="confidence"
						data-k="value"
						size="xxs"
						tone="f2"
						className="w-11 shrink-0 text-right font-mono"
					>
						--
					</Typography.Mono>
				</Flex.Row>
			</Button>
		</Component>
	);
};
