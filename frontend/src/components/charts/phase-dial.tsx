import { createStore } from "@tanstack/store";
import { createRef, useEffect } from "react";
import {
	clearCanvas,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import {
	phaseColumnsFromScan,
	phaseLeadersFromScan,
	type TerminalPhaseResponse,
	type TerminalPhaseStatus,
	type TerminalWaveMode,
} from "#/components/terminal/charts-frame";

/*
The phase dial is the system's own rotation of the universe fingerprint. Angle
is α from the corpus sweep. Radius is signed response mapped so -1 sits at the
center, 0 on the half-radius zero ring, and +1 on the rim. The geodesic under
the dial is the ranked corpus at every α — what the rotation actually saw.
*/

const phaseDialCanvasRef = createRef<HTMLCanvasElement>();

type PhaseDialState = {
	wave: TerminalWaveMode[];
	scan: TerminalPhaseResponse[];
	status: TerminalPhaseStatus;
};

const phaseDialStore = createStore<PhaseDialState>({
	wave: [],
	scan: [],
	status: { ready: false, reason: "" },
});

/*
Sectors are coloured by what price actually did over the outcome horizon, not by
any classifier's opinion of it.
*/
const phaseOutcomeColor = (direction: string): string => {
	if (direction === "up") {
		return TERMINAL_COLORS.green;
	}

	if (direction === "down") {
		return TERMINAL_COLORS.red;
	}

	if (direction === "flat") {
		return TERMINAL_COLORS.cyan;
	}

	return TERMINAL_COLORS.muted;
};

/*
drawPhaseAxes draws the reference rim, the zero ring, and the cardinal spokes
labelled in degrees so a rotation can be read off the dial directly.
*/
const drawPhaseAxes = (
	context: CanvasRenderingContext2D,
	centerX: number,
	centerY: number,
	radius: number,
) => {
	context.strokeStyle = "rgba(105, 177, 203, 0.34)";
	context.lineWidth = 1;
	context.beginPath();
	context.arc(centerX, centerY, radius, 0, Math.PI * 2);
	context.stroke();

	context.strokeStyle = "rgba(147, 138, 126, 0.28)";
	context.beginPath();
	context.arc(centerX, centerY, radius / 2, 0, Math.PI * 2);
	context.stroke();

	context.beginPath();
	context.moveTo(centerX - radius, centerY);
	context.lineTo(centerX + radius, centerY);
	context.moveTo(centerX, centerY - radius);
	context.lineTo(centerX, centerY + radius);
	context.stroke();

	context.strokeStyle = "rgba(147, 138, 126, 0.22)";
	context.beginPath();

	for (let degrees = 0; degrees < 360; degrees += 15) {
		const angle = (degrees * Math.PI) / 180;
		const inner = degrees % 45 === 0 ? radius - 7 : radius - 4;
		context.moveTo(
			centerX + Math.cos(angle) * inner,
			centerY - Math.sin(angle) * inner,
		);
		context.lineTo(
			centerX + Math.cos(angle) * radius,
			centerY - Math.sin(angle) * radius,
		);
	}

	context.stroke();

	context.fillStyle = TERMINAL_COLORS.muted;
	context.font = "9px JetBrains Mono, monospace";
	context.textAlign = "center";
	context.textBaseline = "middle";

	for (const degrees of [0, 90, 180, 270]) {
		const angle = (degrees * Math.PI) / 180;
		context.fillText(
			`${degrees}°`,
			centerX + Math.cos(angle) * (radius + 12),
			centerY - Math.sin(angle) * (radius + 12),
		);
	}

	context.textAlign = "left";
	context.textBaseline = "alphabetic";
};

/*
drawPhaseResponse draws each class-owned sector and its signed response without
joining different winners into one unexplained amber envelope.
*/
const drawPhaseResponse = (
	context: CanvasRenderingContext2D,
	centerX: number,
	centerY: number,
	radius: number,
	scan: TerminalPhaseResponse[],
) => {
	for (const [index, response] of scan.entries()) {
		const next = scan[(index + 1) % scan.length];
		const nextAngle =
			index === scan.length - 1 ? next.angle + Math.PI * 2 : next.angle;
		const similarity = Math.min(1, Math.max(-1, response.similarity));
		const nextSimilarity = Math.min(1, Math.max(-1, next.similarity));
		const responseRadius = (radius * (similarity + 1)) / 2;
		const nextRadius = (radius * (nextSimilarity + 1)) / 2;
		context.strokeStyle = phaseOutcomeColor(response.outcome.direction);
		context.lineWidth = 1.5;
		context.beginPath();
		context.moveTo(
			centerX + Math.cos(response.angle) * responseRadius,
			centerY - Math.sin(response.angle) * responseRadius,
		);
		context.lineTo(
			centerX + Math.cos(nextAngle) * nextRadius,
			centerY - Math.sin(nextAngle) * nextRadius,
		);
		context.stroke();
		context.lineWidth = 3;
		context.beginPath();
		context.arc(centerX, centerY, radius, -response.angle, -nextAngle, true);
		context.stroke();
	}
};

/*
drawPhaseModes plots the resident omega fingerprint after the system's
alignment rotation, the same e^{iα} the sweep applied to the query.
*/
const drawPhaseModes = (
	context: CanvasRenderingContext2D,
	centerX: number,
	centerY: number,
	radius: number,
	wave: TerminalWaveMode[],
	amplitude: number,
	rotation: number,
) => {
	context.fillStyle = TERMINAL_COLORS.cyan;

	for (const mode of wave) {
		const magnitude = Math.hypot(mode.real, mode.imaginary) / amplitude;
		const phase = Math.atan2(mode.imaginary, mode.real) + rotation;
		const pointRadius = radius * magnitude;
		context.fillRect(
			centerX + Math.cos(phase) * pointRadius - 1.5,
			centerY - Math.sin(phase) * pointRadius - 1.5,
			3,
			3,
		);
	}
};

/*
drawPhaseGeodesic paints the ranked corpus against the system's α path. Columns
are rotations, rows are rank, fill is signed response tinted by what that
history then did.
*/
const drawPhaseGeodesic = (
	context: CanvasRenderingContext2D,
	left: number,
	top: number,
	width: number,
	height: number,
	columns: TerminalPhaseResponse[][],
	alignment: TerminalPhaseResponse | null,
) => {
	if (columns.length === 0 || height < 4) {
		return;
	}

	const ranks = Math.max(...columns.map((column) => column.length));
	const cellWidth = width / columns.length;
	const cellHeight = height / ranks;

	context.fillStyle = "rgba(43, 37, 30, 0.65)";
	context.fillRect(left, top, width, height);

	for (const [columnIndex, column] of columns.entries()) {
		for (const [rank, response] of column.entries()) {
			const similarity = Math.min(1, Math.max(-1, response.similarity));
			context.fillStyle = phaseOutcomeColor(response.outcome.direction);
			context.globalAlpha = 0.18 + 0.82 * Math.abs(similarity);
			context.fillRect(
				left + columnIndex * cellWidth,
				top + rank * cellHeight,
				Math.max(1, cellWidth),
				Math.max(1, cellHeight),
			);

			if (similarity < 0) {
				context.fillStyle = "rgba(14, 12, 10, 0.45)";
				context.fillRect(
					left + columnIndex * cellWidth,
					top + rank * cellHeight,
					Math.max(1, cellWidth),
					Math.max(1, cellHeight),
				);
			}
		}
	}

	context.globalAlpha = 1;

	if (alignment === null) {
		return;
	}

	const alignmentIndex = columns.findIndex(
		(column) => column[0]?.angle === alignment.angle,
	);

	if (alignmentIndex < 0) {
		return;
	}

	context.strokeStyle = TERMINAL_COLORS.amber;
	context.lineWidth = 1;
	const mark = left + (alignmentIndex + 0.5) * cellWidth;
	context.beginPath();
	context.moveTo(mark, top);
	context.lineTo(mark, top + height);
	context.stroke();
};

const strongestResponse = (
	scan: TerminalPhaseResponse[],
): TerminalPhaseResponse | null =>
	scan.reduce<TerminalPhaseResponse | null>(
		(best, response) =>
			best === null || response.similarity > best.similarity ? response : best,
		null,
	);

/*
drawPhaseReadout writes what the system sees at its alignment: the leading
history, then the next corpus hits at that same α.
*/
const drawPhaseReadout = (
	context: CanvasRenderingContext2D,
	centerX: number,
	baseline: number,
	alignment: TerminalPhaseResponse | null,
	column: TerminalPhaseResponse[],
	status: TerminalPhaseStatus,
) => {
	context.textAlign = "center";
	context.font = "10px JetBrains Mono, monospace";

	if (!status.ready || alignment === null) {
		context.fillStyle = TERMINAL_COLORS.muted;
		context.fillText(
			status.reason || "waiting for phase scan",
			centerX,
			baseline,
		);
		context.textAlign = "left";
		return;
	}

	const degrees = (alignment.angle * 180) / Math.PI;
	context.fillStyle = phaseOutcomeColor(alignment.outcome.direction);
	context.fillText(
		`${alignment.outcome.direction} · α ${degrees.toFixed(0)}° · ρ ${alignment.similarity.toFixed(2)}`,
		centerX,
		baseline,
	);
	context.fillStyle = TERMINAL_COLORS.muted;
	context.font = "9px JetBrains Mono, monospace";
	const seen = column
		.slice(0, 3)
		.map(
			(response) =>
				`${response.outcome.direction} ${response.similarity.toFixed(2)}`,
		)
		.join(" · ");
	context.fillText(seen, centerX, baseline + 12);
	context.textAlign = "left";
};

const drawPhaseDial = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	state: PhaseDialState,
) => {
	clearCanvas(context, width, height);

	const columns = phaseColumnsFromScan(state.scan);
	const leaders = phaseLeadersFromScan(state.scan);
	const alignment = strongestResponse(leaders);
	const alignedColumn =
		columns.find((column) => column[0]?.angle === alignment?.angle) ?? [];
	const centerX = width / 2;
	// Chrome owns the title rows; the geodesic and readout sit under the dial
	// so the rotation the system already computed stays in the same plate.
	const top = 46;
	const readout = 32;
	const geodesic = Math.max(28, Math.min(64, Math.floor(height * 0.22)));
	const bottom = height - readout - geodesic - 8;
	const centerY = (top + bottom) / 2;
	const radius = Math.max(
		18,
		Math.min((bottom - top) / 2 - 14, width / 2 - 34),
	);

	context.save();
	drawPhaseAxes(context, centerX, centerY, radius);

	if (leaders.length > 1) {
		drawPhaseResponse(context, centerX, centerY, radius, leaders);
	}

	const amplitude = Math.max(
		0,
		...state.wave.map((mode) => Math.hypot(mode.real, mode.imaginary)),
	);

	if (alignment !== null && leaders.length > 1) {
		context.strokeStyle = TERMINAL_COLORS.amber;
		context.lineWidth = 1.5;
		context.beginPath();
		context.moveTo(centerX, centerY);
		context.lineTo(
			centerX + Math.cos(alignment.angle) * radius,
			centerY - Math.sin(alignment.angle) * radius,
		);
		context.stroke();
		context.fillStyle = phaseOutcomeColor(alignment.outcome.direction);
		context.beginPath();
		context.arc(
			centerX + Math.cos(alignment.angle) * radius,
			centerY - Math.sin(alignment.angle) * radius,
			3.5,
			0,
			Math.PI * 2,
		);
		context.fill();
	}

	if (amplitude > 0) {
		drawPhaseModes(
			context,
			centerX,
			centerY,
			radius,
			state.wave,
			amplitude,
			alignment?.angle ?? 0,
		);
	}

	drawPhaseGeodesic(
		context,
		18,
		bottom + 6,
		width - 36,
		geodesic,
		columns,
		alignment,
	);
	drawPhaseReadout(
		context,
		centerX,
		height - 18,
		alignment,
		alignedColumn,
		state.status,
	);
	context.restore();
};

const repaint = () => {
	const canvas = phaseDialCanvasRef.current;

	if (canvas === null) {
		return;
	}

	const context = resizeCanvas(canvas);

	if (context === null) {
		return;
	}

	drawPhaseDial(
		context,
		canvas.clientWidth,
		canvas.clientHeight,
		phaseDialStore.state,
	);
};

/*
paintPhaseDial retains the latest scan so a resize or a remount repaints the same
cut instead of blanking until the next manifold batch.
*/
export const paintPhaseDial = (state: PhaseDialState) => {
	phaseDialStore.setState(() => state);
	repaint();
};

export const TerminalPhaseDialChart = () => {
	useEffect(() => {
		const canvas = phaseDialCanvasRef.current;

		if (canvas === null) {
			return;
		}

		repaint();
		const observer = new ResizeObserver(repaint);
		observer.observe(canvas);
		const subscription = phaseDialStore.subscribe(repaint);

		return () => {
			observer.disconnect();
			subscription.unsubscribe();
		};
	}, []);

	return <canvas ref={phaseDialCanvasRef} className="block size-full" />;
};
