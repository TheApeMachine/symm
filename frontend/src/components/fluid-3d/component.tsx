import { useEffect, useRef, useState } from "react";
import { paintPhaseDial } from "#/components/charts/phase-dial";
import {
	TerminalPhaseDialChart,
	terminalPhaseScanFromFrame,
	terminalPhaseStatusFromFrame,
	terminalWaveModesFromFrame,
} from "#/components/terminal/charts";
import { finiteNumber } from "#/components/terminal/charts-frame";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Canvas } from "#/components/ui/canvas";
import { Flex } from "#/components/ui/flex";
import { Input } from "#/components/ui/input";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { KuramotoRing, type KuramotoRingProps } from "./kuramoto-ring";
import {
	finitePortraitPoint,
	PhasePortrait,
	type PhasePortraitPoint,
} from "./phase-portrait";
import { FluidScene, type FluidSceneOptions } from "./scene";
import { FluidWebRTCFeed } from "./transport";
import type { FluidGrid, FluidParticle } from "./wire";

const initialOptions: FluidSceneOptions = {
	particles: true,
	gas: true,
	wave: true,
	volume: true,
	slices: false,
	exposure: 1.5,
};

// Range compression (see field-shaders.ts `compress`) already lifts faint
// structure into visibility, so exposure only needs a modest ceiling to
// avoid blowing out dense regions.
const maximumVisualExposure = 4;

const kuramotoFromWave = (frame: Record<string, unknown>) => {
	const wave = terminalWaveModesFromFrame(frame);
	let peak = 0;
	const phasors = wave.map((mode) => {
		const magnitude = Math.hypot(mode.real, mode.imaginary);
		peak = Math.max(peak, magnitude);
		return {
			phase: Math.atan2(mode.imaginary, mode.real),
			heat: magnitude,
		};
	});

	return phasors.map((phasor) => ({
		phase: phasor.phase,
		heat: peak > 0 ? phasor.heat / peak : 0,
	}));
};

const Toggle = ({
	active,
	children,
	onClick,
}: {
	active: boolean;
	children: React.ReactNode;
	onClick: () => void;
}) => (
	<Button
		variant={active ? "solid" : "outline"}
		tone={active ? "accent" : "muted"}
		size="xs"
		onClick={onClick}
	>
		{children}
	</Button>
);

const Slider = ({
	label,
	value,
	onChange,
}: {
	label: string;
	value: number;
	onChange: (value: number) => void;
}) => (
	<Flex.Row align="center" gap={2} className="min-w-28">
		<Typography.Label size="xxs" tone="f4" className="w-3">
			{label}
		</Typography.Label>
		<Input
			type="range"
			min={0}
			max={1}
			step={0.001}
			value={value}
			className="h-4 w-20 accent-(--acc)"
			onChange={(event) => onChange(event.currentTarget.valueAsNumber)}
		/>
	</Flex.Row>
);

const particleReadout = (particle: FluidParticle | null) => {
	if (particle === null) {
		return "click an order to inspect its geometric (particle) and wave (oscillator) resident state";
	}

	const pos = particle.Position;
	const vel = particle.Velocity;
	const mass = particle.Mass;
	const heat = particle.Heat;
	const energy = particle.Energy;
	const phase = particle.Phase;
	const omega = particle.Omega;
	const amp = particle.Amplitude;
	const real = amp * Math.cos(phase);
	const imag = amp * Math.sin(phase);

	return [
		"── GEOMETRIC DOMAIN (PARTICLE) ──",
		`pos  [${pos.X.toFixed(4)}, ${pos.Y.toFixed(4)}, ${pos.Z.toFixed(4)}]`,
		`vel  [${vel.X.toFixed(4)}, ${vel.Y.toFixed(4)}, ${vel.Z.toFixed(4)}]`,
		`mass ${mass.toPrecision(4)} · heat ${heat.toPrecision(4)} · energy ${energy.toPrecision(4)}`,
		"── WAVE DOMAIN (OSCILLATOR) ──",
		`phase ${phase.toFixed(4)} rad (${((phase * 180) / Math.PI).toFixed(1)}°) · ω ${omega.toFixed(4)}`,
		`amplitude ${amp.toPrecision(4)} · phasor (${real.toFixed(4)} + ${imag.toFixed(4)}i)`,
	].join("\n");
};

/*
FluidInspector is the live 3D diagnostic surface for the resident particle gas,
Eulerian gas fields, and complex spatial wave field.
*/
export const FluidInspector = () => {
	const viewportRef = useRef<HTMLDivElement>(null);
	const sceneRef = useRef<FluidScene | null>(null);
	const feedRef = useRef<FluidWebRTCFeed | null>(null);
	const [state, setState] = useState<RTCPeerConnectionState | "connecting">(
		"connecting",
	);
	const [error, setError] = useState<string | null>(null);
	const [grid, setGrid] = useState<FluidGrid | null>(null);
	const [particleCount, setParticleCount] = useState(0);
	const [selected, setSelected] = useState<FluidParticle | null>(null);
	const [options, setOptions] = useState(initialOptions);
	const [slices, setSlices] = useState({ x: 0.5, y: 0.5, z: 0.5 });
	const [kuramotoProps, setKuramotoProps] = useState<KuramotoRingProps>({
		oscillators: [],
		kuramotoR: 0,
		kuramotoPsi: 0,
	});
	const phaseHistoryRef = useRef<PhasePortraitPoint[]>([]);
	const [phasePortrait, setPhasePortrait] = useState<{
		history: PhasePortraitPoint[];
		current: PhasePortraitPoint;
	}>({
		history: [],
		current: { divergence: 0, pressureGradNorm: 0 },
	});

	const [hydro, setHydro] = useState<Record<string, number> | null>(null);

	const connect = () => {
		setError(null);
		void feedRef.current?.connect();
	};

	useEffect(() => {
		if (viewportRef.current === null) {
			return;
		}

		let scene: FluidScene;

		try {
			scene = new FluidScene(viewportRef.current, setSelected, (cause) =>
				setError(cause.message),
			);
		} catch (cause) {
			setError(cause instanceof Error ? cause.message : String(cause));
			return;
		}

		const feed = new FluidWebRTCFeed({
			onFields: (fields) => {
				scene.updateFields(fields);
				setGrid(fields.grid);
			},
			onParticles: (particles) => {
				scene.updateParticles(particles);
				setParticleCount(particles.count);
			},
			onPhase: (frame) => {
				paintPhaseDial({
					wave: terminalWaveModesFromFrame(frame),
					scan: terminalPhaseScanFromFrame(frame),
					status: terminalPhaseStatusFromFrame(frame),
				});

				const hydrodynamics = frame.hydrodynamics as
					| Record<string, number>
					| undefined;

				if (hydrodynamics !== undefined) {
					setHydro(hydrodynamics);
					setKuramotoProps({
						oscillators: kuramotoFromWave(frame),
						kuramotoR: finiteNumber(hydrodynamics.kuramotoR) ?? 0,
						kuramotoPsi: finiteNumber(hydrodynamics.kuramotoPsi) ?? 0,
					});

					const point = finitePortraitPoint(
						hydrodynamics.divergence,
						hydrodynamics.pressureGradNorm,
					);

					if (point !== null) {
						const history = phaseHistoryRef.current;
						history.push(point);

						if (history.length > 200) {
							history.splice(0, history.length - 200);
						}

						setPhasePortrait({ history: [...history], current: point });
					}
				}
			},
			onState: setState,
			onError: (cause) => setError(cause.message),
		});
		sceneRef.current = scene;
		feedRef.current = feed;
		scene.setOptions(initialOptions);
		void feed.connect();

		return () => {
			feed.close();
			scene.dispose();
			feedRef.current = null;
			sceneRef.current = null;
		};
	}, []);

	useEffect(() => {
		sceneRef.current?.setOptions(options);
	}, [options]);

	useEffect(() => {
		sceneRef.current?.setSlices(slices.x, slices.y, slices.z);
	}, [slices]);

	const toggle = (key: keyof Omit<FluidSceneOptions, "exposure">) => {
		setOptions((current) => ({ ...current, [key]: !current[key] }));
	};

	const statusVariant = state === "connected" ? "success" : "warning";

	return (
		<Section className="relative size-full min-h-0" surface="sunken">
			<Section.Header
				title="Fluid manifold · 3D inspection"
				meta={
					<Flex.Row align="center" gap={3}>
						<Badge label={state} variant={statusVariant} size="xs" dot />
						<Typography.Mono size="s" tone="f4">
							{grid === null
								? "waiting for fields"
								: `${grid.x}×${grid.y}×${grid.z} · ${particleCount} orders/particles`}
						</Typography.Mono>
						{hydro !== null ? (
							<Flex.Row
								align="center"
								gap={2}
								className="text-[11px] text-(--f4)"
							>
								<span>η: {hydro.viscosityProxy?.toFixed(3) ?? "0"}</span>
								<span>v_B: {hydro.guidanceSpeed?.toFixed(3) ?? "0"}</span>
								<span>⟨|Ψ|²⟩: {hydro.coherenceMag2?.toFixed(3) ?? "0"}</span>
							</Flex.Row>
						) : null}
					</Flex.Row>
				}
				className="absolute inset-x-0 top-0 z-10 bg-[#0e0c0ae8]"
			>
				<Flex.Row gap={2} align="center" className="ml-2">
					<Toggle
						active={options.particles}
						onClick={() => toggle("particles")}
					>
						particles
					</Toggle>
					<Toggle active={options.gas} onClick={() => toggle("gas")}>
						gas
					</Toggle>
					<Toggle active={options.wave} onClick={() => toggle("wave")}>
						wave
					</Toggle>
					<Toggle active={options.volume} onClick={() => toggle("volume")}>
						volume
					</Toggle>
					<Toggle active={options.slices} onClick={() => toggle("slices")}>
						slices
					</Toggle>
					<Slider
						label="α"
						value={Math.min(options.exposure / maximumVisualExposure, 1)}
						onChange={(value) =>
							setOptions((current) => ({
								...current,
								exposure: value * maximumVisualExposure,
							}))
						}
					/>
				</Flex.Row>
			</Section.Header>

			<div ref={viewportRef} className="absolute inset-0" />

			{options.slices ? (
				<Flex.Column
					gap={2}
					className="absolute top-14 right-3 z-10 rounded border border-(--line) bg-[#0e0c0ae8] p-2"
				>
					<Slider
						label="X"
						value={slices.x}
						onChange={(x) => setSlices((current) => ({ ...current, x }))}
					/>
					<Slider
						label="Y"
						value={slices.y}
						onChange={(y) => setSlices((current) => ({ ...current, y }))}
					/>
					<Slider
						label="Z"
						value={slices.z}
						onChange={(z) => setSlices((current) => ({ ...current, z }))}
					/>
				</Flex.Column>
			) : null}

			<Typography.Pre className="absolute bottom-3 left-3 z-10 m-0 whitespace-pre rounded border border-(--line) bg-[#0e0c0ae8] p-2 text-[10px] leading-4 text-(--f3)">
				{particleReadout(selected)}
			</Typography.Pre>

			{/* Kuramoto sync ring */}
			<div className="absolute right-3 bottom-3 z-10 h-44 w-44 rounded border border-(--line) bg-[#0e0c0ae8] p-1">
				<Typography.Label size="xxs" tone="f4" className="mb-0.5 text-center">
					Kuramoto sync
				</Typography.Label>
				<KuramotoRing {...kuramotoProps} />
			</div>

			{/* Hydrodynamic phase portrait */}
			<div className="absolute right-50 bottom-3 z-10 h-44 w-56 rounded border border-(--line) bg-[#0e0c0ae8] p-1">
				<Typography.Label size="xxs" tone="f4" className="mb-0.5 text-center">
					∇·u vs ‖∇P‖
				</Typography.Label>
				<PhasePortrait {...phasePortrait} />
			</div>

			<Canvas
				title="Phase dial"
				meta="system α sweep · ranked corpus geodesic · realized direction"
				topRight={
					<div className="flex flex-col gap-0.5">
						<span className="inline-flex items-center justify-end gap-1.5">
							<span className="inline-block size-1.5 bg-(--acc)" />
							alignment ray
						</span>
						<span className="inline-flex items-center justify-end gap-1.5">
							<span className="inline-block size-1.5 bg-info" />
							wave modes
						</span>
						<span className="inline-flex items-center justify-end gap-1.5">
							<span className="inline-block h-px w-3 bg-(--line2)" />ρ = 0 ring
						</span>
						<span className="inline-flex items-center justify-end gap-1.5">
							<span className="inline-block h-1.5 w-3 bg-(--acc)" />
							corpus × α
						</span>
					</div>
				}
				className="absolute top-14 left-3 z-10 h-104 w-80 rounded border border-(--line) bg-[#0e0c0ae8]"
			>
				<TerminalPhaseDialChart />
			</Canvas>

			{error === null ? null : (
				<Flex.Row
					align="center"
					gap={3}
					className="absolute right-3 bottom-3 z-10 rounded border border-(--error) bg-[#0e0c0af2] p-2"
				>
					<Typography.Mono size="s" className="text-(--error)">
						{error}
					</Typography.Mono>
					<Button variant="outline" tone="error" size="xs" onClick={connect}>
						reconnect
					</Button>
				</Flex.Row>
			)}
		</Section>
	);
};
