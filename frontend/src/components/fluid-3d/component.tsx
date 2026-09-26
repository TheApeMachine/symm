import { useEffect, useRef, useState } from "react";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Input } from "#/components/ui/input";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { KuramotoRing } from "./kuramoto-ring";
import { FluidScene, type FluidSceneOptions } from "./scene";
import { FluidParticleFrame, type FluidParticle } from "./wire";

export interface NativeManifoldFrame {
	epoch: string;
	sequence: string;
	version: string;
	gridX: number;
	gridY: number;
	gridZ: number;
	spacing: number;
	population: number;
	positions: number[];
	velocities: number[];
	masses: number[];
	energies: number[];
	phases: number[];
	frequencies: number[];
	amplitudes: number[];
	heat: number[];
	densityMomentum: number[];
	fieldEnergy: number[];
	waveReal: number[];
	waveImaginary: number[];
	densityScale: number;
	momentumScale: number;
	energyScale: number;
	waveScale: number;
	divergence: number;
	guidanceSpeed: number;
	coherence: number;
	pressureGradient: number;
	viscosity: number;
	synchronization: number;
	physicalTime: number;
	acceptedStep: number;
	substeps: number;
}
export interface FluidInspectorProps {
	frame?: NativeManifoldFrame;
	className?: string;
}
const initialOptions: FluidSceneOptions = {
	particles: true,
	current: true,
	gas: true,
	wave: true,
	volume: true,
	slices: false,
	exposure: 1.5,
};

/* FluidInspector receives the retained physical node's native frame. The scene
 owns presentation only; field state and transport remain in the authored graph. */
export const FluidInspector = ({ frame, className }: FluidInspectorProps) => {
	const viewport = useRef<HTMLDivElement>(null);
	const scene = useRef<FluidScene | null>(null);
	const [error, setError] = useState<string>();
	const [selected, setSelected] = useState<FluidParticle | null>(null);
	const [options, setOptions] = useState(initialOptions);
	const [slices, setSlices] = useState({ x: 0.5, y: 0.5, z: 0.5 });
	useEffect(() => {
		if (!viewport.current) return;
		try {
			scene.current = new FluidScene(viewport.current, setSelected, (cause) =>
				setError(cause.message),
			);
		} catch (cause) {
			setError(cause instanceof Error ? cause.message : String(cause));
			return;
		}
		return () => {
			scene.current?.dispose();
			scene.current = null;
		};
	}, []);
	useEffect(() => {
		if (!frame || !scene.current) return;
		const sequence = BigInt(frame.sequence);
		scene.current.updateFields({
			sequence,
			grid: {
				x: frame.gridX,
				y: frame.gridY,
				z: frame.gridZ,
				spacing: frame.spacing,
			},
			momRho: Float32Array.from(frame.densityMomentum),
			internalEnergy: Float32Array.from(frame.fieldEnergy),
			waveReal: Float32Array.from(frame.waveReal),
			waveImaginary: Float32Array.from(frame.waveImaginary),
			densityScale: frame.densityScale,
			momentumScale: frame.momentumScale,
			energyScale: frame.energyScale,
			waveScale: frame.waveScale,
		});
		scene.current.updateParticles(
			new FluidParticleFrame(
				sequence,
				frame.population,
				Float32Array.from(frame.positions),
				Float32Array.from(frame.velocities),
				Float32Array.from(frame.masses),
				Float32Array.from(frame.heat),
				Float32Array.from(frame.energies),
				Float32Array.from(frame.phases),
				Float32Array.from(frame.frequencies),
				Float32Array.from(frame.amplitudes),
			),
		);
	}, [frame]);
	useEffect(() => {
		scene.current?.setOptions(options);
	}, [options]);
	useEffect(() => {
		scene.current?.setSlices(slices.x, slices.y, slices.z);
	}, [slices]);
	const meanPhase = frame
		? Math.atan2(
				frame.phases.reduce((sum, phase) => sum + Math.sin(phase), 0),
				frame.phases.reduce((sum, phase) => sum + Math.cos(phase), 0),
			)
		: 0;
	const frequencyExtent = frame
		? frame.frequencies.reduce(
				(maximum, frequency) => Math.max(maximum, Math.abs(frequency)),
				0,
			)
		: 0;
	return (
		<Section
			className={`relative h-full min-h-96 ${className ?? ""}`}
			surface="sunken"
		>
			<div ref={viewport} className="absolute inset-0" />
			<Flex.Column
				className="absolute inset-x-0 top-0 z-10 border-b border-(--line) bg-(--surface) p-3"
				gap={2}
			>
				<Flex.Row gap={3} align="center">
					<Typography.Label>Resident fluid manifold</Typography.Label>
					<Badge
						label={
							frame ? `${frame.population} orders` : "Awaiting physical frame"
						}
						variant={frame ? "success" : "warning"}
					/>
					<Typography.Mono>
						epoch {frame?.epoch ?? "—"} · sequence {frame?.sequence ?? "—"} ·{" "}
						{frame ? `${frame.gridX}×${frame.gridY}×${frame.gridZ}` : "—"}
					</Typography.Mono>
				</Flex.Row>
				<Flex.Row gap={2} align="center">
					{(
						["particles", "current", "gas", "wave", "volume", "slices"] as const
					).map((key) => (
						<Button
							key={key}
							size="xs"
							variant={options[key] ? "solid" : "outline"}
							onClick={() =>
								setOptions((current) => ({ ...current, [key]: !current[key] }))
							}
						>
							{key}
						</Button>
					))}
				</Flex.Row>
				{frame && (
					<Typography.Mono className="text-xs">
						∇·u {frame.divergence.toPrecision(4)} · guidance{" "}
						{frame.guidanceSpeed.toPrecision(4)} · coherence{" "}
						{frame.coherence.toPrecision(4)} · ∇P{" "}
						{frame.pressureGradient.toPrecision(4)} · viscosity{" "}
						{frame.viscosity.toPrecision(4)} · time{" "}
						{frame.physicalTime.toPrecision(5)} · accepted Δt{" "}
						{frame.acceptedStep.toPrecision(4)} / {frame.substeps} steps
					</Typography.Mono>
				)}
				{options.slices && (
					<Flex.Row gap={3}>
						{(["x", "y", "z"] as const).map((axis) => (
							<label key={axis}>
								{axis}
								<Input
									type="range"
									min={0}
									max={1}
									step={0.001}
									value={slices[axis]}
									onChange={(event) =>
										setSlices((current) => ({
											...current,
											[axis]: event.currentTarget.valueAsNumber,
										}))
									}
								/>
							</label>
						))}
					</Flex.Row>
				)}
			</Flex.Column>
			{frame && (
				<div className="absolute bottom-3 right-3 h-44 w-44 bg-(--surface) p-2">
					<Typography.Label>Oscillator synchronization</Typography.Label>
					<KuramotoRing
						oscillators={frame.phases.map((phase, index) => ({
							phase,
							heat:
								frequencyExtent > 0
									? Math.abs(frame.frequencies[index]!) / frequencyExtent
									: 0,
						}))}
						kuramotoR={frame.synchronization}
						kuramotoPsi={meanPhase}
					/>
				</div>
			)}
			<Typography.Pre className="absolute bottom-3 left-3 bg-(--surface) p-3 text-xs">
				{selected
					? `Position ${Object.values(selected.Position)
							.map((value) => value.toPrecision(4))
							.join(" · ")}\nVelocity ${Object.values(selected.Velocity)
							.map((value) => value.toPrecision(4))
							.join(
								" · ",
							)}\nMass ${selected.Mass.toPrecision(4)} · heat ${selected.Heat.toPrecision(4)} · energy ${selected.Energy.toPrecision(4)}\nPhase ${selected.Phase.toPrecision(4)} · frequency ${selected.Omega.toPrecision(4)}`
					: "Select an order particle to inspect its integrated state"}
			</Typography.Pre>
			{error && (
				<Typography.Mono className="absolute inset-x-3 bottom-3 bg-(--surface) p-3 text-(--error)">
					{error}
				</Typography.Mono>
			)}
		</Section>
	);
};
