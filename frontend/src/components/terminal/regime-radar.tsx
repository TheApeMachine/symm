import { Flex } from "@/components/ui/flex";
import { Panel } from "@/components/ui/panel";
import {Component} from "#/components/ui/component";
import { cn } from "#/lib/utils";

/*
RadarPanel is the static regime-radar shell. DRAW paints via paintRegimeRadar.
*/
export const RadarPanel = () => (
	<Component registerKey="">
		{({ ref, className }) => (
			<div ref={ref} className={cn("flex h-full flex-col", className)}>
				<Panel size="lg">
					<Flex className="mb-2 font-semibold text-(--f1) text-xs">
						Regime radar
					</Flex>
					<Flex className="mb-2 font-mono text-[9.5px] text-(--f4)">
						cross-section mean · market
					</Flex>
					<svg viewBox="0 0 220 210" className="block w-full">
						<title>Regime radar</title>
						<polygon
							points="110,21 190,79 159,173 61,173 30,79"
							fill="none"
							stroke="#3a342b"
						/>
						<polygon
							points="110,49 163,87 142,154 78,154 57,87"
							fill="none"
							stroke="#2b251e"
						/>
						<polygon
							points="110,77 137,94 126,134 94,134 83,94"
							fill="none"
							stroke="#2b251e"
						/>
							<line
								key={`${x}:${y}`}
								x1="110"
								y1="105"
								x2={110 + x * 84}
								y2={105 + y * 84}
								stroke="#2b251e"
							/>
						<polygon
							fill="rgba(232,163,61,0.22)"
							stroke="#e8a33d"
							strokeWidth="1.6"
						/>
							<text
								key={`${x}:${y}`}
								x={110 + x * 98}
								y={105 + y * 98}
								textAnchor="middle"
								fontSize="9"
								fill="#938a7e"
							/>
					</svg>
				</Panel>
			</div>
		)}
	</Component>
);
