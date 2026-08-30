import { useEffect, useRef } from "react";
import { cognitionStore } from "#/collections/app";
import {
	CortexLeafRoster,
	drawCortexTree,
} from "#/components/terminal/cortex-draw";
import { cortexTreeFromReading } from "#/components/terminal/cortex-tree";
import { EnvelopeCognition } from "#/providers/telemetry/telemetry/envelope-cognition";
import { EnvelopeCognitionBeam } from "#/providers/telemetry/telemetry/envelope-cognition-beam";
import { EnvelopeCognitionBranch } from "#/providers/telemetry/telemetry/envelope-cognition-branch";
import { EnvelopeCognitionClass } from "#/providers/telemetry/telemetry/envelope-cognition-class";

const branchObj = new EnvelopeCognitionBranch();
const beamObj = new EnvelopeCognitionBeam();
const classObj = new EnvelopeCognitionClass();

const cognitionToRecord = (cog: EnvelopeCognition | null): Record<string, unknown> | null => {
	if (!cog) return null;

	const branches: any[] = [];
	for (let i = 0; i < cog.branchesLength(); i++) {
		const b = cog.branches(i, branchObj);
		if (b) {
			branches.push({
				id: Number(b.id()),
				parentId: Number(b.parentId()),
				token: b.token() ?? "node",
				prefix: b.prefix() ?? "",
				key: b.key() ?? "",
				depth: Number(b.depth()),
				probability: b.probability(),
				count: Number(b.count()),
			});
		}
	}

	const beams: any[] = [];
	for (let i = 0; i < cog.beamsLength(); i++) {
		const b = cog.beams(i, beamObj);
		if (b) {
			beams.push({
				sequence: b.sequence() ?? "",
				key: b.key() ?? "",
				score: b.score(),
			});
		}
	}

	const classes: any[] = [];
	for (let i = 0; i < cog.classesLength(); i++) {
		const c = cog.classes(i, classObj);
		if (c) {
			classes.push({
				name: c.name() ?? "",
				probability: c.probability(),
			});
		}
	}

	return {
		beamWidth: Number(cog.beamWidth()),
		maxHops: Number(cog.maxHops()),
		nodeCount: Number(cog.nodeCount()),
		branches,
		beams,
		classes,
	};
};

/*
CortexCanvas draws the sensory prefix tree.
*/
export const CortexCanvas = ({
	symbol,
	className,
}: {
	symbol: string;
	className?: string;
}) => {
	const canvasRef = useRef<HTMLCanvasElement>(null);
	const rosterRef = useRef(new CortexLeafRoster());
	const readingRef = useRef<Record<string, unknown> | null>(null);

	useEffect(() => {
		const draw = () => {
			const canvas = canvasRef.current;
			if (canvas === null) return;

			const width = Math.max(1, canvas.clientWidth);
			const height = Math.max(1, canvas.clientHeight);
			const ratio = window.devicePixelRatio || 1;

			if (
				canvas.width !== Math.floor(width * ratio) ||
				canvas.height !== Math.floor(height * ratio)
			) {
				canvas.width = Math.floor(width * ratio);
				canvas.height = Math.floor(height * ratio);
			}

			const context = canvas.getContext("2d");
			const tree = cortexTreeFromReading(readingRef.current);

			if (context === null) return;

			context.setTransform(ratio, 0, 0, ratio, 0, 0);

			if (tree === null) {
				context.clearRect(0, 0, width, height);
				return;
			}

			drawCortexTree(context, width, height, tree, rosterRef.current);
		};

		const paint = (record: Record<string, unknown> | null): void => {
			readingRef.current = record;
			draw();
		};

		const subscription = cognitionStore.subscribe((state) => {
			const last = state.getLast();
			if (!last) return;

			const targetRow: EnvelopeCognition | null =
				typeof last.symbol() === "string" && last.symbol() === symbol
					? last
					: null;

			if (!targetRow) return;
			paint(cognitionToRecord(targetRow));
		});

		const observer = new ResizeObserver(draw);
		const canvas = canvasRef.current;

		if (canvas !== null) {
			observer.observe(canvas);
		}

		draw();

		return () => {
			subscription.unsubscribe();
			observer.disconnect();
		};

	}, [symbol]);

	return <canvas ref={canvasRef} className={className} />;
};