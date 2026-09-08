import { useSelector } from "@tanstack/react-store";
import { learningStore } from "#/collections/learning";
import { onlineStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Panel } from "#/components/ui/panel";

const Row = ({
	label,
	children,
}: {
	label: string;
	children: React.ReactNode;
}) => (
	<Flex.Row justify="between" align="center" className="gap-2">
		<Flex className="shrink-0 text-(--f4)">{label}</Flex>
		{children}
	</Flex.Row>
);

export const Engine = () => {
	const learning = useSelector(learningStore, (state) => state);
	const online = useSelector(onlineStore, (state) => state === "ONLINE");
	const policy = learning?.agents[0];

	return (
		<Panel size="bare" className="p-2.5 font-mono text-[11px] leading-[1.7]">
			<Row label="observations">
				<Flex data-e="seq" className="text-(--f1)">
					{String(learning?.steps ?? "—")}
				</Flex>
			</Row>
			<Row label="phase">
				<Flex data-e="phase" className="min-w-0 truncate text-(--acc)">
					{online ? (learning?.status ?? "waiting") : "offline"}
				</Flex>
			</Row>
			<Row label="decisions">
				<Flex data-e="cand" className="text-(--f1)">
					{String(learning?.decisions ?? "—")}
				</Flex>
			</Row>
			<Row label="graded">
				<Flex data-e="meas" className="text-(--f1)">
					{String(learning?.resolved ?? "—")}
				</Flex>
			</Row>
			<Row label="open">
				<Flex data-e="open" className="text-(--f1)">
					{String(
						policy?.positions.filter(
							(position) => Number(position.holding?.qty) > 0,
						).length ?? "—",
					)}
				</Flex>
			</Row>
		</Panel>
	);
};
