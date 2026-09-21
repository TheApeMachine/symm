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

export const Engine = () => {
	const online = useSelector(onlineAtom);
	const phase = useSelector(phaseAtom);
	const seq = useSelector(tickCountAtom);
	const cand = useSelector(candidatesAtom);
	const open = useSelector(positionCountAtom);
	const displayPhase = online === "OFFLINE" ? "offline" : String(phase ?? "—");

	return (
		<Panel size="bare" className="p-2.5 font-mono text-[11px] leading-[1.7]">
			<Flex.Row justify="between" align="center" gap={2}>
				<Flex.Row className="shrink-0 text-(--f4)">observations</Flex.Row>
				<Flex.Row data-e="seq" className="text-(--f1)">
					{String(seq ?? "—")}
				</Flex.Row>
			</Flex.Row>
			<Flex.Row>
				<Flex.Row className="shrink-0 text-(--f4)">phase</Flex.Row>
				<Flex.Row data-e="phase" className="min-w-0 truncate text-(--acc)">
					{displayPhase}
				</Flex.Row>
			</Flex.Row>
			<Flex.Row>
				<Flex.Row className="shrink-0 text-(--f4)">decisions</Flex.Row>
				<Flex.Row data-e="cand" className="text-(--f1)">
					{String(cand ?? "—")}
				</Flex.Row>
			</Flex.Row>
			<Flex.Row>
				<Flex.Row className="shrink-0 text-(--f4)">open</Flex.Row>
				<Flex.Row data-e="open" className="text-(--f1)">
					{String(open ?? "—")}
				</Flex.Row>
			</Flex.Row>
		</Panel>
	);
};
