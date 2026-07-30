import { Flex } from "../ui/flex";
import type { LayoutType } from "./layout-textures";

interface NodeGraphLegacyLayoutControlsProps {
	currentLayout: LayoutType;
	onApplyLayout: (layout: LayoutType) => void;
	onFitCamera: () => void;
	isDofEnabled: boolean;
	onToggleDof: () => void;
	autoFitCamera: boolean;
	onToggleAutoFit: () => void;
}

export function NodeGraphLegacyLayoutControls({
	autoFitCamera,
	currentLayout,
	isDofEnabled,
	onApplyLayout,
	onFitCamera,
	onToggleAutoFit,
	onToggleDof,
}: NodeGraphLegacyLayoutControlsProps) {
	return (
		<Flex className="absolute top-0 right-0 z-10">
			<button
				type="button"
				onClick={onFitCamera}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					borderRadius: 6,
					color: "#fff",
					cursor: "pointer",
					fontFamily: "Monospace",
					fontSize: 12,
					height: 32,
					lineHeight: "32px",
					outline: "1px solid rgba(255,255,255,0.25)",
					padding: 0,
					textAlign: "center",
					width: 32,
				}}
				title="Fit camera to graph"
			>
				FIT
			</button>

			<button
				type="button"
				onClick={onToggleDof}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					borderRadius: 6,
					color: "#fff",
					cursor: "pointer",
					fontFamily: "Monospace",
					fontSize: 12,
					height: 32,
					lineHeight: "32px",
					opacity: isDofEnabled ? 1 : 0.5,
					outline: "1px solid rgba(255,255,255,0.25)",
					padding: 0,
					textAlign: "center",
					width: 32,
				}}
				title="Depth of Field"
			>
				DOF
			</button>

			<button
				type="button"
				onClick={onToggleAutoFit}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					borderRadius: 6,
					color: "#fff",
					cursor: "pointer",
					fontFamily: "Monospace",
					fontSize: 12,
					height: 32,
					lineHeight: "32px",
					opacity: autoFitCamera ? 1 : 0.5,
					outline: "1px solid rgba(255,255,255,0.25)",
					padding: 0,
					textAlign: "center",
					width: 32,
				}}
				title="Auto-fit camera on graph/layout changes"
			>
				AUTO
			</button>

			<button
				type="button"
				onClick={() => onApplyLayout("force")}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					cursor: "pointer",
					opacity: currentLayout === "force" ? 1 : 0.5,
					padding: 0,
				}}
				title="Force Directed"
			>
				<img
					src="/textures/forceDirected.png"
					width="32"
					height="32"
					alt="Force"
				/>
			</button>

			<button
				type="button"
				onClick={() => onApplyLayout("spherical")}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					cursor: "pointer",
					opacity: currentLayout === "spherical" ? 1 : 0.5,
					padding: 0,
				}}
				title="Spherical"
			>
				<img
					src="/textures/sphere.png"
					width="32"
					height="32"
					alt="Spherical"
				/>
			</button>

			<button
				type="button"
				onClick={() => onApplyLayout("helix")}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					cursor: "pointer",
					opacity: currentLayout === "helix" ? 1 : 0.5,
					padding: 0,
				}}
				title="Helix"
			>
				<img src="/textures/spring.png" width="32" height="32" alt="Helix" />
			</button>

			<button
				type="button"
				onClick={() => onApplyLayout("grid3d")}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					cursor: "pointer",
					opacity: currentLayout === "grid3d" ? 1 : 0.5,
					padding: 0,
				}}
				title="Grid 3D"
			>
				<img src="/textures/square.png" width="32" height="32" alt="Grid 3D" />
			</button>

			<button
				type="button"
				onClick={() => onApplyLayout("cylinder")}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					borderRadius: 6,
					color: "#fff",
					cursor: "pointer",
					fontFamily: "Monospace",
					fontSize: 12,
					height: 32,
					lineHeight: "32px",
					opacity: currentLayout === "cylinder" ? 1 : 0.5,
					outline: "1px solid rgba(255,255,255,0.25)",
					padding: 0,
					textAlign: "center",
					width: 32,
				}}
				title="Cylinder (token ring × layer stack)"
			>
				CYL
			</button>

			<button
				type="button"
				onClick={() => onApplyLayout("radialLayered")}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					borderRadius: 6,
					color: "#fff",
					cursor: "pointer",
					fontFamily: "Monospace",
					fontSize: 12,
					height: 32,
					lineHeight: "32px",
					opacity: currentLayout === "radialLayered" ? 1 : 0.5,
					outline: "1px solid rgba(255,255,255,0.25)",
					padding: 0,
					textAlign: "center",
					width: 32,
				}}
				title="Radial layered (rings = layers)"
			>
				RAD
			</button>

			<button
				type="button"
				onClick={() => onApplyLayout("bfs3d")}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					borderRadius: 6,
					color: "#fff",
					cursor: "pointer",
					fontFamily: "Monospace",
					fontSize: 10,
					height: 32,
					lineHeight: "32px",
					opacity: currentLayout === "bfs3d" ? 1 : 0.5,
					outline: "1px solid rgba(255,255,255,0.25)",
					padding: 0,
					textAlign: "center",
					width: 32,
				}}
				title="BFS 3D (stacked planes)"
			>
				B3D
			</button>

			<button
				type="button"
				onClick={() => onApplyLayout("radialBfs")}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					borderRadius: 6,
					color: "#fff",
					cursor: "pointer",
					fontFamily: "Monospace",
					fontSize: 11,
					height: 32,
					lineHeight: "32px",
					opacity: currentLayout === "radialBfs" ? 1 : 0.5,
					outline: "1px solid rgba(255,255,255,0.25)",
					padding: 0,
					textAlign: "center",
					width: 32,
				}}
				title="Radial BFS (rings = BFS depth)"
			>
				RBFS
			</button>

			<button
				type="button"
				onClick={() => onApplyLayout("tree3d")}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					borderRadius: 6,
					color: "#fff",
					cursor: "pointer",
					fontFamily: "Monospace",
					fontSize: 10,
					height: 32,
					lineHeight: "32px",
					opacity: currentLayout === "tree3d" ? 1 : 0.5,
					outline: "1px solid rgba(255,255,255,0.25)",
					padding: 0,
					textAlign: "center",
					width: 32,
				}}
				title="Tree 3D (stacked planes)"
			>
				T3D
			</button>

			<button
				type="button"
				onClick={() => onApplyLayout("dag3d")}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					borderRadius: 6,
					color: "#fff",
					cursor: "pointer",
					fontFamily: "Monospace",
					fontSize: 10,
					height: 32,
					lineHeight: "32px",
					opacity: currentLayout === "dag3d" ? 1 : 0.5,
					outline: "1px solid rgba(255,255,255,0.25)",
					padding: 0,
					textAlign: "center",
					width: 32,
				}}
				title="DAG 3D (stacked planes)"
			>
				D3D
			</button>

			<button
				type="button"
				onClick={() => onApplyLayout("guided3d")}
				style={{
					appearance: "none",
					background: "transparent",
					border: "none",
					borderRadius: 6,
					color: "#fff",
					cursor: "pointer",
					fontFamily: "Monospace",
					fontSize: 10,
					height: 32,
					lineHeight: "32px",
					opacity: currentLayout === "guided3d" ? 1 : 0.5,
					outline: "1px solid rgba(255,255,255,0.25)",
					padding: 0,
					textAlign: "center",
					width: 32,
				}}
				title="Guided 3D force (BFS planes + full 3D forces)"
			>
				G3D
			</button>
		</Flex>
	);
}
