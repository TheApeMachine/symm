import { useSelector } from "@tanstack/react-store";
import {
	candidatesAtom,
	onlineAtom,
	phaseAtom,
	positionCountAtom,
	tickCountAtom,
} from "#/collections/app";
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
	const online = useSelector(onlineAtom, (state) => state === "ONLINE");
	const phase = useSelector(phaseAtom, (state) => state);
	const seq = useSelector(tickCountAtom, (state) => state);
	const cand = useSelector(candidatesAtom, (state) => state);
	const open = useSelector(positionCountAtom, (state) => state);

	let phaseText = "offline";
	if (online) {
		phaseText = phase || "waiting";
	}

	return (
		<Panel size="bare" className="p-2.5 font-mono text-[11px] leading-[1.7]">
			<Row label="observations">
				<Flex data-e="seq" className="text-(--f1)">
					{String(seq ?? "—")}
				</Flex>
			</Row>
			<Row label="phase">
				<Flex data-e="phase" className="min-w-0 truncate text-(--acc)">
					{phaseText}
				</Flex>
			</Row>
			<Row label="decisions">
				<Flex data-e="cand" className="text-(--f1)">
					{String(cand ?? "—")}
				</Flex>
			</Row>
			<Row label="open">
				<Flex data-e="open" className="text-(--f1)">
					{String(open ?? "—")}
				</Flex>
			</Row>
		</Panel>
	);
};
