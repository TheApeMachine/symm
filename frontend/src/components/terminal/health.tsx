import { Badge } from "@/components/ui/badge";
import { Flex } from "@/components/ui/flex";
import { Meter } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import { Stat } from "@/components/ui/stat";
import { Component } from "#/components/ui/component";

/*
HealthPanel is the static system-health shell. DRAW paints via
paintHealthMeasurements and paintHealthTick.
*/
export const HealthPanel = () => (
	<Component registerKey="health">
		{({ ref, className }) => (
			<Panel size="lg" ref={ref} className={className}>
				<Flex.Row align="center" justify="between">
					<Flex className="font-semibold text-(--f1) text-xs">
						System health
					</Flex>
					<Badge
						label=""
						className="rounded-[3px] border px-1.5 py-0.5 font-mono text-[9px] font-semibold uppercase tracking-wide"
					/>
				</Flex.Row>
				<Flex.Row className="mt-3 gap-4.5">
					<Stat
						value=""
						label="tick"
						emphasis="default"
						valueClassName="font-normal"
					/>
					<Stat
						value=""
						label="healthy"
						emphasis="default"
						valueClassName="font-normal text-(--f1)"
					/>
					<Stat
						value=""
						label="avg strength"
						emphasis="default"
						valueClassName="font-normal text-(--f1)"
					/>
					<Stat
						value=""
						label="firing"
						emphasis="default"
						valueClassName="font-normal"
					/>
				</Flex.Row>
				<Flex.Column className="mt-3.25 gap-1.5">
					<Meter layout="inline" size="xs" percent={0} label="" value="" />
					<Meter layout="inline" size="xs" percent={0} label="" value="" />
					<Meter layout="inline" size="xs" percent={0} label="" value="" />
				</Flex.Column>
			</Panel>
		)}
	</Component>
);
