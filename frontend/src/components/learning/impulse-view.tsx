import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "#/components/ui/button";
import { ToggleGroup, ToggleGroupItem } from "#/components/ui/toggle-group";
import { Typography } from "#/components/ui/typography";

export interface ImpulsePoint {
	id: number;
	source: string;
	label: string;
	cluster: number;
	snr: number;
	activation: number;
	x: number;
	y: number;
	vx?: number;
	vy?: number;
	gridX: number;
	gridY: number;
	energy: number;
	authority: number;
	present: boolean;
}

export interface HotRegion {
	id: number;
	source: string;
	snr: number;
	authority: number;
	members: number;
}

const generateInitialSignals = (count = 120): ImpulsePoint[] => {
	const points: ImpulsePoint[] = [];
	const cols = 15;
	const rows = Math.ceil(count / cols);

	for (let i = 0; i < count; i++) {
		const cluster = i % 4;
		const col = i % cols;
		const row = Math.floor(i / cols);
		const gridX = (col - (cols - 1) / 2) * 44;
		const gridY = (row - (rows - 1) / 2) * 40;

		points.push({
			id: i + 1,
			source: `sig_${(i + 1).toString().padStart(3, "0")}`,
			label: `CELL_${col}_${row}`,
			cluster,
			snr: 1.5 + Math.pow(Math.random(), 2.5) * 8.5,
			activation: Math.random() * 0.2,
			x: gridX,
			y: gridY,
			vx: 0,
			vy: 0,
			gridX,
			gridY,
			energy: 0.1,
			authority: 0.5 + Math.random() * 0.5,
			present: true,
		});
	}
	return points;
};

export interface ImpulseViewProps {
	livePoints?: ImpulsePoint[];
	liveRegions?: HotRegion[];
}

interface Point2D {
	x: number;
	y: number;
}

interface ContourLoop {
	points: Point2D[];
}

interface ContourLevelData {
	level: number;
	threshold: number;
	loops: ContourLoop[];
}

// Marching Squares density contour generator
function computeDensityContours(
	nodes: ImpulsePoint[],
	viewWidth: number,
	viewHeight: number,
	cellSize = 16,
	levels = 12,
): ContourLevelData[] {
	const cols = Math.ceil(viewWidth / cellSize) + 1;
	const rows = Math.ceil(viewHeight / cellSize) + 1;
	const halfW = viewWidth / 2;
	const halfH = viewHeight / 2;

	// Allocate and zero grid
	const grid: Float32Array[] = new Array(rows);
	for (let r = 0; r < rows; r++) {
		grid[r] = new Float32Array(cols);
	}

	// Gaussian density splatting (bandwidth ~34px)
	const sigma = 34;
	const twoSigma2 = 2 * sigma * sigma;
	const maxR = 2.4 * sigma;
	const maxR2 = maxR * maxR;

	let maxVal = 0;
	for (const node of nodes) {
		const weight = node.activation * node.snr;
		if (weight < 0.03) continue;

		const gx = (node.x + halfW) / cellSize;
		const gy = (node.y + halfH) / cellSize;
		const rCells = Math.ceil(maxR / cellSize);

		const minC = Math.max(0, Math.floor(gx - rCells));
		const maxC = Math.min(cols - 1, Math.ceil(gx + rCells));
		const minR = Math.max(0, Math.floor(gy - rCells));
		const maxR_ = Math.min(rows - 1, Math.ceil(gy + rCells));

		for (let r = minR; r <= maxR_; r++) {
			const dy = (r - gy) * cellSize;
			const dy2 = dy * dy;
			for (let c = minC; c <= maxC; c++) {
				const dx = (c - gx) * cellSize;
				const d2 = dx * dx + dy2;
				if (d2 < maxR2) {
					const val = (grid[r][c] += weight * Math.exp(-d2 / twoSigma2));
					if (val > maxVal) maxVal = val;
				}
			}
		}
	}

	if (maxVal < 0.08) return [];

	const contourLevels: ContourLevelData[] = [];

	const interp = (
		valA: number,
		valB: number,
		posA: number,
		posB: number,
		thresh: number,
	) => {
		if (Math.abs(valB - valA) < 1e-6) return (posA + posB) / 2;
		return posA + ((thresh - valA) / (valB - valA)) * (posB - posA);
	};

	// Generate isolines for each threshold level
	for (let i = 1; i <= levels; i++) {
		const threshold = maxVal * (i / (levels + 1));
		const segments: [Point2D, Point2D][] = [];

		for (let r = 0; r < rows - 1; r++) {
			for (let c = 0; c < cols - 1; c++) {
				const v0 = grid[r][c];
				const v1 = grid[r][c + 1];
				const v2 = grid[r + 1][c + 1];
				const v3 = grid[r + 1][c];

				let code = 0;
				if (v0 >= threshold) code |= 1;
				if (v1 >= threshold) code |= 2;
				if (v2 >= threshold) code |= 4;
				if (v3 >= threshold) code |= 8;

				if (code === 0 || code === 15) continue;

				const x = c * cellSize - halfW;
				const y = r * cellSize - halfH;

				const top: Point2D = {
					x: interp(v0, v1, x, x + cellSize, threshold),
					y,
				};
				const right: Point2D = {
					x: x + cellSize,
					y: interp(v1, v2, y, y + cellSize, threshold),
				};
				const bot: Point2D = {
					x: interp(v3, v2, x, x + cellSize, threshold),
					y: y + cellSize,
				};
				const left: Point2D = {
					x,
					y: interp(v0, v3, y, y + cellSize, threshold),
				};

				switch (code) {
					case 1:
					case 14:
						segments.push([left, top]);
						break;
					case 2:
					case 13:
						segments.push([top, right]);
						break;
					case 3:
					case 12:
						segments.push([left, right]);
						break;
					case 4:
					case 11:
						segments.push([right, bot]);
						break;
					case 5: {
						const avg = (v0 + v1 + v2 + v3) / 4;
						if (avg >= threshold) {
							segments.push([left, top]);
							segments.push([bot, right]);
						} else {
							segments.push([left, bot]);
							segments.push([top, right]);
						}
						break;
					}
					case 6:
					case 9:
						segments.push([top, bot]);
						break;
					case 7:
					case 8:
						segments.push([left, bot]);
						break;
					case 10: {
						const avg = (v0 + v1 + v2 + v3) / 4;
						if (avg >= threshold) {
							segments.push([top, right]);
							segments.push([left, bot]);
						} else {
							segments.push([left, top]);
							segments.push([right, bot]);
						}
						break;
					}
				}
			}
		}

		// Stitch segments into continuous loops
		const loops: ContourLoop[] = [];
		const unused = [...segments];
		const tol = cellSize * 0.75;

		while (unused.length > 0) {
			const first = unused.pop();
			if (!first) break;
			const loopPoints: Point2D[] = [first[0], first[1]];

			let extended = true;
			while (extended && unused.length > 0) {
				extended = false;
				const tip = loopPoints[loopPoints.length - 1];
				const tail = loopPoints[0];

				for (let s = 0; s < unused.length; s++) {
					const [p1, p2] = unused[s];
					const dTip1 = Math.hypot(tip.x - p1.x, tip.y - p1.y);
					const dTip2 = Math.hypot(tip.x - p2.x, tip.y - p2.y);
					const dTail1 = Math.hypot(tail.x - p1.x, tail.y - p1.y);
					const dTail2 = Math.hypot(tail.x - p2.x, tail.y - p2.y);

					if (dTip1 < tol) {
						loopPoints.push(p2);
						unused.splice(s, 1);
						extended = true;
						break;
					}
					if (dTip2 < tol) {
						loopPoints.push(p1);
						unused.splice(s, 1);
						extended = true;
						break;
					}
					if (dTail2 < tol) {
						loopPoints.unshift(p1);
						unused.splice(s, 1);
						extended = true;
						break;
					}
					if (dTail1 < tol) {
						loopPoints.unshift(p2);
						unused.splice(s, 1);
						extended = true;
						break;
					}
				}
			}

			if (loopPoints.length >= 3) {
				loops.push({ points: loopPoints });
			}
		}

		contourLevels.push({
			level: i,
			threshold,
			loops,
		});
	}

	return contourLevels;
}

export const ImpulseView = ({
	livePoints,
	liveRegions,
}: ImpulseViewProps) => {
	const canvasRef = useRef<HTMLCanvasElement>(null);
	const wrapperRef = useRef<HTMLDivElement>(null);
	const [layoutMode, setLayoutMode] = useState<"grid" | "regions">("grid");
	const [isPlaying, setIsPlaying] = useState(true);

	const nodesRef = useRef<ImpulsePoint[]>(generateInitialSignals());
	const layoutModeRef = useRef<"grid" | "regions">("grid");
	layoutModeRef.current = layoutMode;

	const isPlayingRef = useRef(true);
	isPlayingRef.current = isPlaying;

	const [hoveredNode, setHoveredNode] = useState<{
		node: ImpulsePoint;
		x: number;
		y: number;
	} | null>(null);

	const [recentPulses] = useState([
		{ id: 1, name: "Orderbook Imbalance", latency: "12ms", hot: true },
		{ id: 2, name: "ETH/USD Vol Break", latency: "38ms", hot: false },
		{ id: 3, name: "BTC Momentum Shift", latency: "94ms", hot: false },
	]);

	// Update nodes from livePoints if telemetry arrives
	useEffect(() => {
		if (livePoints && livePoints.length > 0) {
			nodesRef.current = livePoints.map((p) => ({
				...p,
				vx: 0,
				vy: 0,
			}));
		}
	}, [livePoints]);

	// High-performance Canvas Animation Loop (60/120 FPS via requestAnimationFrame)
	useEffect(() => {
		const canvas = canvasRef.current;
		const wrapper = wrapperRef.current;
		if (!canvas || !wrapper) return;

		const ctx = canvas.getContext("2d");
		if (!ctx) return;

		let animationFrameId: number;
		let lastTapePulse = performance.now();
		let contourAlpha = 0.0; // Smooth fade between grid and regions

		const clusterFoci = [
			{ x: -140, y: -90 },
			{ x: 140, y: -90 },
			{ x: -140, y: 90 },
			{ x: 140, y: 90 },
		];

		const render = (time: number) => {
			const width = wrapper.clientWidth;
			const height = wrapper.clientHeight;
			const dpr = window.devicePixelRatio || 1;

			if (canvas.width !== width * dpr || canvas.height !== height * dpr) {
				canvas.width = width * dpr;
				canvas.height = height * dpr;
			}

			ctx.save();
			ctx.scale(dpr, dpr);
			ctx.clearRect(0, 0, width, height);

			const centerX = width / 2;
			const centerY = height / 2;
			const currentMode = layoutModeRef.current;
			const playing = isPlayingRef.current;
			const nodes = nodesRef.current;

			// Smooth contour fade (in for regions, out for grid)
			const targetContourAlpha = currentMode === "regions" ? 1.0 : 0.0;
			contourAlpha += (targetContourAlpha - contourAlpha) * 0.06;

			// Tape impulse injection every ~1.2s
			if (playing && time - lastTapePulse > 1200) {
				lastTapePulse = time;
				const targetCluster = Math.floor(Math.random() * 4);
				for (const node of nodes) {
					if (node.cluster === targetCluster && Math.random() < 0.6) {
						node.activation = Math.min(
							1.0,
							node.activation + Math.random() * 0.5 + 0.3,
						);
					}
				}
			}

			// Damped Physics Step: Critically damped spring prevents rubbery bounce
			const k = 0.045; // attraction rate
			const damping = 0.32; // critical damping prevents oscillation

			for (const node of nodes) {
				let targetX = node.gridX;
				let targetY = node.gridY;

				if (currentMode === "regions") {
					const focus = clusterFoci[node.cluster] ?? { x: 0, y: 0 };
					const angle = ((node.id * 37) % 360) * (Math.PI / 180);
					const dist = 32 + (node.id % 45);
					targetX = focus.x + Math.cos(angle) * dist;
					targetY = focus.y + Math.sin(angle) * dist;
				}

				if (playing) {
					const ax = (targetX - node.x) * k - (node.vx ?? 0) * damping;
					const ay = (targetY - node.y) * k - (node.vy ?? 0) * damping;
					node.vx = (node.vx ?? 0) + ax;
					node.vy = (node.vy ?? 0) + ay;
					node.x += node.vx;
					node.y += node.vy;

					// Activation decay
					if (node.activation > 0.04) {
						node.activation = Math.max(0.02, node.activation - 0.005);
					}
				} else {
					node.x = targetX;
					node.y = targetY;
					node.vx = 0;
					node.vy = 0;
				}
			}

			ctx.translate(centerX, centerY);

			// Compute & Draw Multi-Level Topographic Contours (Otsu split & elevation bands from mockup)
			if (contourAlpha > 0.02) {
				const contourLevels = computeDensityContours(nodes, width, height, 18, 12);

				for (const levelData of contourLevels) {
					const lvl = levelData.level;
					const isOtsuSplit = lvl === 5; // Reaction threshold boundary (Otsu equivalent)
					const isHotPeak = lvl === 9; // High-confluence hot boundary

					ctx.save();
					ctx.globalAlpha = contourAlpha;

					for (const loop of levelData.loops) {
						if (loop.points.length < 3) continue;

						ctx.beginPath();
						ctx.moveTo(loop.points[0].x, loop.points[0].y);
						for (let p = 1; p < loop.points.length; p++) {
							ctx.lineTo(loop.points[p].x, loop.points[p].y);
						}
						ctx.closePath();

						// Elevation fill shading
						ctx.fillStyle = `rgba(232, 163, 61, ${Math.min(0.22, lvl * 0.015)})`;
						ctx.fill();

						// Topographic boundary strokes
						if (isOtsuSplit) {
							// Otsu split reaction boundary in distinct green
							ctx.strokeStyle = "rgba(156, 192, 110, 0.75)";
							ctx.lineWidth = 1.6;
							ctx.stroke();
						} else if (isHotPeak) {
							// Hot peak boundary in bright gold
							ctx.strokeStyle = "rgba(232, 163, 61, 0.9)";
							ctx.lineWidth = 1.8;
							ctx.stroke();
						} else if (lvl % 2 === 1) {
							// Subtle elevation isolines
							ctx.strokeStyle = "rgba(232, 163, 61, 0.16)";
							ctx.lineWidth = 0.8;
							ctx.stroke();
						}
					}
					ctx.restore();
				}
			}

			// Draw Sympathy Connections between hot nodes in the same cluster
			const hotThreshold = 0.42;
			ctx.lineWidth = 1.5;
			ctx.strokeStyle = "#e8a33d";

			for (let i = 0; i < nodes.length; i++) {
				const na = nodes[i];
				if (na.activation < hotThreshold) continue;

				for (let j = i + 1; j < nodes.length; j++) {
					const nb = nodes[j];
					if (nb.activation < hotThreshold || na.cluster !== nb.cluster) continue;

					const dx = na.x - nb.x;
					const dy = na.y - nb.y;
					const dist = Math.sqrt(dx * dx + dy * dy);

					if (dist < 90) {
						ctx.globalAlpha = Math.min(0.8, (na.activation + nb.activation) / 2);
						ctx.beginPath();
						ctx.moveTo(na.x, na.y);
						ctx.lineTo(nb.x, nb.y);
						ctx.stroke();
					}
				}
			}

			// Draw Nodes
			for (const node of nodes) {
				const r = Math.max(3, Math.min(8.5, node.snr * 0.9));
				const isHot = node.activation > 0.4;
				const isWarm = node.activation > 0.15;

				// Glowing halo on hot nodes
				if (isHot) {
					ctx.save();
					ctx.globalAlpha = node.activation * 0.35;
					ctx.fillStyle = "#e8a33d";
					ctx.beginPath();
					ctx.arc(node.x, node.y, r * 2.2, 0, Math.PI * 2);
					ctx.fill();
					ctx.restore();
				}

				ctx.beginPath();
				ctx.arc(node.x, node.y, r, 0, Math.PI * 2);

				if (isHot) {
					ctx.fillStyle = "#e8a33d";
					ctx.globalAlpha = 0.95;
				} else if (isWarm) {
					ctx.fillStyle = "#7fbacb";
					ctx.globalAlpha = 0.65;
				} else {
					ctx.fillStyle = "#1f1a14";
					ctx.globalAlpha = 0.45;
				}
				ctx.fill();

				ctx.strokeStyle = isHot ? "#f4efe5" : "#2b251e";
				ctx.lineWidth = isHot ? 1.4 : 0.8;
				ctx.globalAlpha = 0.8;
				ctx.stroke();
			}

			ctx.restore();
			animationFrameId = requestAnimationFrame(render);
		};

		animationFrameId = requestAnimationFrame(render);
		return () => cancelAnimationFrame(animationFrameId);
	}, []);

	// Mouse hover picking for tooltip
	const handleMouseMove = (e: React.MouseEvent<HTMLCanvasElement>) => {
		const canvas = canvasRef.current;
		const wrapper = wrapperRef.current;
		if (!canvas || !wrapper) return;

		const rect = canvas.getBoundingClientRect();
		const mouseX = e.clientX - rect.left - wrapper.clientWidth / 2;
		const mouseY = e.clientY - rect.top - wrapper.clientHeight / 2;

		let closest: ImpulsePoint | null = null;
		let minDist = 18;

		for (const node of nodesRef.current) {
			const dx = node.x - mouseX;
			const dy = node.y - mouseY;
			const dist = Math.sqrt(dx * dx + dy * dy);
			if (dist < minDist) {
				minDist = dist;
				closest = node;
			}
		}

		if (closest) {
			setHoveredNode({
				node: closest,
				x: e.clientX,
				y: e.clientY,
			});
		} else {
			setHoveredNode(null);
		}
	};

	// Hot regions
	const displayRegions = useMemo(() => {
		if (liveRegions && liveRegions.length > 0) return liveRegions;

		const clusterWeights = [0, 1, 2, 3].map((c) => {
			const cNodes = nodesRef.current.filter((n) => n.cluster === c);
			const snr = cNodes.reduce((s, n) => s + n.activation * n.snr, 0) / (cNodes.length || 1);
			const names = [
				"MOMENTUM CONFLUENCE",
				"ORDERBOOK IMBALANCE",
				"VOLATILITY EXPANSION",
				"MEAN REVERSION BIAS",
			];
			return {
				id: c + 1,
				source: names[c] ?? `REGION_${c}`,
				snr: Math.max(1.2, snr * 2.5),
				authority: 0.72 + c * 0.06,
				members: cNodes.length,
			};
		});

		return clusterWeights.sort((a, b) => b.snr - a.snr);
	}, [liveRegions]);

	const maxRegionSnr = Math.max(...displayRegions.map((r) => r.snr), 1);

	return (
		<div className="flex h-full w-full min-h-0 flex-col overflow-hidden bg-(--bg) font-mono text-xs text-(--f2)">
			{/* Top Controls Toolbar - Flush with border-b */}
			<div className="flex h-10 shrink-0 items-center justify-between border-b border-(--line) bg-(--surface) px-4">
				<div className="flex items-center gap-3">
					<Typography.Label size="s" tone="f4" weight="normal">
						TOPOLOGY
					</Typography.Label>
					<ToggleGroup
						name="layout"
						value={layoutMode}
						onValueChange={(val) => setLayoutMode(val as typeof layoutMode)}
					>
						<ToggleGroupItem value="grid">Initial Grid</ToggleGroupItem>
						<ToggleGroupItem value="regions">Sympathy Clustering</ToggleGroupItem>
					</ToggleGroup>

					<div className="h-4 w-px bg-(--line)" />

					<div className="flex items-center gap-4 text-[10px] text-(--f3)">
						<div className="flex items-center gap-1.5">
							<span className="h-2 w-2 rounded-full bg-(--up)" />
							<span>Action Boundary (Otsu Split)</span>
						</div>
						<div className="flex items-center gap-1.5">
							<span className="h-2 w-2 rounded-full bg-(--acc)" />
							<span>Hot Peak (SNR Confluence)</span>
						</div>
					</div>
				</div>

				<div className="flex items-center gap-3">
					<Button
						variant="outline"
						size="s"
						onClick={() => {
							for (const n of nodesRef.current) {
								n.x = n.gridX;
								n.y = n.gridY;
								n.vx = 0;
								n.vy = 0;
								n.activation = 0;
							}
						}}
					>
						RESET
					</Button>
					<Button
						variant="outline"
						size="s"
						onClick={() => setIsPlaying(!isPlaying)}
						className="text-(--acc)"
					>
						{isPlaying ? "PAUSE" : "RESUME"}
					</Button>
				</div>
			</div>

			{/* Main Surface Split - Flush edge-to-edge */}
			<div className="flex flex-1 min-h-0 overflow-hidden">
				{/* Left: High-Performance Canvas Screen - Rendered on black screen surface bg-(--sunken) */}
				<div
					ref={wrapperRef}
					className="relative flex flex-1 min-w-0 bg-(--sunken) overflow-hidden"
				>
					{/* Crosshair Center Reference */}
					<div className="absolute inset-0 flex items-center justify-center pointer-events-none opacity-20">
						<div className="h-full w-px bg-(--line)" />
						<div className="absolute w-full h-px bg-(--line)" />
					</div>

					<canvas
						ref={canvasRef}
						onMouseMove={handleMouseMove}
						onMouseLeave={() => setHoveredNode(null)}
						className="absolute inset-0 block h-full w-full cursor-crosshair"
					/>

					{/* Market Tape Feed Overlay HUD */}
					<div className="absolute top-3 left-3 w-56 rounded border border-(--line) bg-(--surface)/90 p-2.5 backdrop-blur shadow-xl pointer-events-none">
						<div className="mb-2 flex items-center justify-between text-[10px] text-(--f4)">
							<span className="font-bold text-(--f1)">MARKET TAPE FEED</span>
							<span className="h-1.5 w-1.5 rounded-full bg-(--up) animate-pulse" />
						</div>
						<div className="flex flex-col gap-1.5 text-[10px]">
							{recentPulses.map((pulse) => (
								<div
									key={pulse.id}
									className="flex items-center justify-between border-b border-(--line)/40 pb-0.5"
								>
									<span className="truncate text-(--f3)">{pulse.name}</span>
									<span className={pulse.hot ? "font-bold text-(--acc)" : "text-(--info)"}>
										{pulse.latency}
									</span>
								</div>
							))}
						</div>
					</div>

					{/* Bottom Status Readout */}
					<div className="absolute bottom-3 left-3 rounded border border-(--line) bg-(--surface)/80 px-2.5 py-1 text-[10px] text-(--f4) backdrop-blur pointer-events-none">
						<span>{nodesRef.current.length} signal cells</span>
						<span className="mx-2">·</span>
						<span>{displayRegions.length} sympathy clusters active</span>
					</div>

					{/* Tooltip on Hover */}
					{hoveredNode && (
						<div
							className="fixed z-50 rounded border border-(--line) bg-(--raised) p-2 font-mono text-[10px] text-(--f2) shadow-xl pointer-events-none"
							style={{ left: hoveredNode.x + 12, top: hoveredNode.y + 12 }}
						>
							<div className="font-bold text-(--acc)">{hoveredNode.node.source}</div>
							<div className="text-(--f4)">{hoveredNode.node.label}</div>
							<div className="mt-1 flex justify-between gap-4">
								<span>SNR:</span>
								<span className="font-bold text-(--f1)">
									{hoveredNode.node.snr.toFixed(2)}
								</span>
							</div>
							<div className="flex justify-between gap-4">
								<span>Activation:</span>
								<span>{(hoveredNode.node.activation * 100).toFixed(0)}%</span>
							</div>
						</div>
					)}
				</div>

				{/* Right: Hot Regions Panel - Flush with border-l */}
				<div className="flex w-80 shrink-0 flex-col border-l border-(--line) bg-(--surface) overflow-hidden min-h-0">
					<div className="flex h-8 shrink-0 items-center justify-between border-b border-(--line) bg-(--surface) px-4 text-[11px] text-(--f4)">
						<span className="font-bold text-(--f2)">HOT REGIONS</span>
						<span>Ranked by SNR</span>
					</div>

					<div className="flex flex-1 flex-col gap-3 p-4 overflow-hidden min-h-0">
						<Typography.Mono size="s" tone="f4" className="text-[10.5px] leading-relaxed">
							Evidenced communities of numeric cells that fire together under market
							impulse. Bar indicates relative signal-to-noise ratio.
						</Typography.Mono>

						<div className="flex flex-1 flex-col gap-3 overflow-hidden pt-1">
							{displayRegions.map((region) => {
								const widthPct = Math.min(
									100,
									Math.max(8, (region.snr / maxRegionSnr) * 100),
								);

								return (
									<div key={region.id} className="flex flex-col gap-1 border-b border-(--line)/40 pb-2.5">
										<div className="flex items-center justify-between text-[11px]">
											<span className="font-bold text-(--f1) truncate">
												{region.source}
											</span>
											<span className="font-mono text-(--acc) font-bold shrink-0">
												{region.snr.toFixed(2)} SNR
											</span>
										</div>

										<div className="h-1.5 w-full overflow-hidden rounded-full bg-(--line)">
											<div
												className="h-full bg-(--acc) transition-[width] duration-300"
												style={{ width: `${widthPct}%` }}
											/>
										</div>

										<div className="flex items-center justify-between text-[9px] text-(--f4)">
											<span>Authority: {(region.authority * 100).toFixed(0)}%</span>
											<span>{region.members} cells</span>
										</div>
									</div>
								);
							})}
						</div>

						<div className="border-t border-(--line) pt-2 text-[10px] text-(--f4)">
							Continuous stream from telemetry grid.
						</div>
					</div>
				</div>
			</div>
		</div>
	);
};
