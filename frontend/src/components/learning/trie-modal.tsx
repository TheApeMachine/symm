import { useEffect, useRef } from "react";
import { CortexLeafRoster, drawCortexTree } from "#/components/terminal/cortex-draw";
import { cortexTreeFromReading } from "#/components/terminal/cortex-tree";
import { Modal } from "#/components/ui/modal";
import { Typography } from "#/components/ui/typography";
import type { LearningLearnerT } from "#/providers/telemetry/telemetry/learning-learner";

/*
TrieModal draws one learner's memory with the same renderer Cortex uses.

The nodes arrive in the shape that renderer already reads, so nothing is
translated here and no second tree drawing exists to drift from the first. What
is shown is what the learner holds: the moments it has learned about, and under
each of them the region sequences that ran into it.
*/
export const TrieModal = ({
	learner,
	open,
	onClose,
}: {
	learner: LearningLearnerT | null;
	open: boolean;
	onClose: () => void;
}) => {
	const canvas = useRef<HTMLCanvasElement>(null);
	const branches = learner?.branches ?? [];

	useEffect(() => {
		const surface = canvas.current;

		if (!open || !surface) {
			return;
		}
		const context = surface.getContext("2d");

		if (!context) {
			return;
		}

		const paint = () => {
			const box = surface.getBoundingClientRect();
			const ratio = window.devicePixelRatio || 1;
			surface.width = Math.max(1, Math.floor(box.width * ratio));
			surface.height = Math.max(1, Math.floor(box.height * ratio));
			context.setTransform(ratio, 0, 0, ratio, 0, 0);

			const tree = cortexTreeFromReading({
				branches: branches.map((branch) => ({
					id: Number(branch?.id ?? 0),
					parentId: Number(branch?.parentId ?? -1),
					token: String(branch?.token ?? ""),
					prefix: String(branch?.prefix ?? ""),
					key: String(branch?.key ?? ""),
					depth: Number(branch?.depth ?? 0),
					probability: branch?.probability ?? 0,
					count: Number(branch?.count ?? 0),
				})),
				beams: [],
			});

			if (tree) {
				drawCortexTree(
					context,
					box.width,
					box.height,
					tree,
					new CortexLeafRoster(),
				);
			}
		};

		paint();
		const observer = new ResizeObserver(paint);
		observer.observe(surface);

		return () => observer.disconnect();
	}, [open, branches]);

	return (
		<Modal
			open={open}
			onClose={onClose}
			size="xl"
			// A tree needs width more than a dialog does; the size variants cap
			// at a reading column, which folds the picture into a column of edges.
			panelClassName="h-[80vh] w-[min(1180px,94vw)] max-w-none"
		>
			<Modal.Header>
				<div>
					<Typography.Label size="m" tone="f2">
						Learner {(learner?.id ?? 0) + 1} · what it holds
					</Typography.Label>
					<Typography.Mono size="s" tone="f4" className="mt-0.5 block">
						{Number(learner?.links ?? 0).toLocaleString()} learned situations ·{" "}
						{branches.length} nodes drawn · edge = how often the sequence ran
						into that moment
					</Typography.Mono>
				</div>
				<Modal.Close onClick={onClose} />
			</Modal.Header>
			<Modal.Body className="min-h-0 p-0">
				{branches.length > 0 ? (
					<canvas
						ref={canvas}
						className="block h-full w-full bg-(--bg)"
						aria-label={`Radix trie held by learner ${(learner?.id ?? 0) + 1}`}
					/>
				) : (
					<Typography.Mono size="s" tone="f3" className="p-4">
						This learner has not committed anything to memory yet.
					</Typography.Mono>
				)}
			</Modal.Body>
		</Modal>
	);
};
