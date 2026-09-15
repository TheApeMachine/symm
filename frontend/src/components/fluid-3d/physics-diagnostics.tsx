import { useState } from "react";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import type { FluidPhaseReading } from "./wire";

type DiagnosticTab = "integrator" | "gas" | "wave" | "residuals";

const formatSci = (num: number | undefined | null, prec = 3): string => {
	if (num === undefined || num === null || Number.isNaN(num)) return "—";
	if (Math.abs(num) < 1e-4 && num !== 0) return num.toExponential(prec);
	return num.toFixed(prec);
};

export const PhysicsDiagnosticsHUD = ({
	isOpen,
	onClose,
	phaseReading,
	particleCount,
}: {
	isOpen: boolean;
	onClose: () => void;
	phaseReading: FluidPhaseReading | null;
	particleCount: number;
}) => {
	const [tab, setTab] = useState<DiagnosticTab>("integrator");

	if (!isOpen) return null;

	const health = phaseReading?.health;
	const integrator = health?.integrator;
	const gas = health?.gas;
	const wave = health?.wave;
	const pilot = health?.pilot;
	const sources = health?.sources;

	const machVal = gas?.maxMach ?? 0;
	const vorticityVal = gas?.vorticityRms ?? 0;
	const strainVal = gas?.strainRms ?? 0;
	const gasKinetic = gas?.kinetic ?? 0;
	const gasInternal = gas?.internal ?? 0;
	const waveNorm = wave?.norm ?? 0;
	const kuramotoR = phaseReading?.kuramotoR ?? 0;
	const divergence = phaseReading?.divergence ?? 0;
	const guidanceSpeed = phaseReading?.guidanceSpeed ?? 0;
	const coherenceMag2 = phaseReading?.coherenceMag2 ?? 0;
	const viscosityProxy = phaseReading?.viscosityProxy ?? 0;
	const totalParticles = particleCount ?? 0;

	const rejections = integrator?.rejections ?? 0;
	const substeps = integrator?.substeps ?? 0;
	const version = phaseReading?.version;

	return (
		<div className="absolute top-14 right-3 z-30 flex max-h-[85vh] w-[460px] flex-col overflow-hidden rounded-lg border border-(--line) bg-[color-mix(in_srgb,var(--bg)_95%,transparent)] shadow-2xl backdrop-blur-md">
			{/* Top Header */}
			<div className="flex items-center justify-between border-b border-(--line) px-3 py-2 bg-[color-mix(in_srgb,var(--bg)_80%,var(--surface))]">
				<Flex.Row align="center" gap={2}>
					<Typography.Label size="xs" weight="semibold" className="text-(--f1)">
						Physics Engine Monitor
					</Typography.Label>
					<Badge
						size="xs"
						variant={rejections > 0 ? "error" : "success"}
						label={rejections > 0 ? `${rejections} REJECTIONS` : "STABLE"}
						dot
					/>
					{version !== undefined && version > 0n ? (
						<Typography.Mono size="xxs" tone="f4">
							v{version.toString()}
						</Typography.Mono>
					) : null}
				</Flex.Row>
				<Flex.Row align="center" gap={2}>
					<Typography.Mono size="xxs" tone="f3">
						{totalParticles} particles
					</Typography.Mono>
					<button
						type="button"
						onClick={onClose}
						className="cursor-pointer text-xs text-(--f4) hover:text-(--f1)"
					>
						✕
					</button>
				</Flex.Row>
			</div>

			{/* Sub-tabs */}
			<div className="flex border-b border-(--line) bg-(--bg) px-2 pt-1 gap-1 text-[11px]">
				<Button
					size="xs"
					variant={tab === "integrator" ? "solid" : "quiet"}
					tone={tab === "integrator" ? "accent" : "muted"}
					onClick={() => setTab("integrator")}
				>
					Integrator & CFL
				</Button>
				<Button
					size="xs"
					variant={tab === "gas" ? "solid" : "quiet"}
					tone={tab === "gas" ? "accent" : "muted"}
					onClick={() => setTab("gas")}
				>
					Gas & Mach
				</Button>
				<Button
					size="xs"
					variant={tab === "wave" ? "solid" : "quiet"}
					tone={tab === "wave" ? "accent" : "muted"}
					onClick={() => setTab("wave")}
				>
					Wave & Pilot
				</Button>
				<Button
					size="xs"
					variant={tab === "residuals" ? "solid" : "quiet"}
					tone={tab === "residuals" ? "accent" : "muted"}
					onClick={() => setTab("residuals")}
				>
					Conservation
				</Button>
			</div>

			{/* Tab Content Body */}
			<div className="flex-1 overflow-y-auto p-3 text-xs">
				{phaseReading === null && (
					<div className="mb-3 rounded border border-(--line) bg-(--surface) p-2 text-center text-[11px] text-(--f4)">
						Waiting for WebRTC manifold stream...
					</div>
				)}

				{tab === "integrator" && (
					<div className="space-y-3 font-mono">
						<div className="grid grid-cols-3 gap-2">
							<div className="rounded border border-(--line) bg-(--surface) p-2">
								<div className="text-[9px] uppercase tracking-wider text-(--f4)">
									Accepted Δt
								</div>
								<div className="text-sm font-semibold text-(--f1)">
									{formatSci(integrator?.acceptedDt, 5)}
								</div>
							</div>
							<div className="rounded border border-(--line) bg-(--surface) p-2">
								<div className="text-[9px] uppercase tracking-wider text-(--f4)">
									Substeps
								</div>
								<div className="text-sm font-semibold text-(--f1)">
									{substeps}
								</div>
							</div>
							<div className="rounded border border-(--line) bg-(--surface) p-2">
								<div className="text-[9px] uppercase tracking-wider text-(--f4)">
									Rejections
								</div>
								<div
									className={`text-sm font-semibold ${
										rejections > 0 ? "text-(--error)" : "text-(--success)"
									}`}
								>
									{rejections}
								</div>
							</div>
						</div>

						<div className="rounded border border-(--line) bg-(--surface) p-2.5">
							<div className="mb-1.5 text-[10px] font-medium uppercase tracking-wider text-(--f3)">
								Adaptive Time-Step Limiters (CFL)
							</div>
							<div className="grid grid-cols-2 gap-x-4 gap-y-1 text-[11px]">
								<div className="flex justify-between">
									<span className="text-(--f4)">Hyperbolic Δt:</span>
									<span className="text-(--f2)">
										{formatSci(integrator?.hyperbolicDt, 5)}
									</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Viscous Δt:</span>
									<span className="text-(--f2)">
										{formatSci(integrator?.viscousDt, 5)}
									</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Particle Δt:</span>
									<span className="text-(--f2)">
										{formatSci(integrator?.particleDt, 5)}
									</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Phase Δt:</span>
									<span className="text-(--f2)">
										{formatSci(integrator?.phaseDt, 5)}
									</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Thermal Δt:</span>
									<span className="text-(--f2)">
										{formatSci(integrator?.thermalDt, 5)}
									</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Combined Δt:</span>
									<span className="text-accent font-semibold">
										{formatSci(integrator?.combinedDt, 5)}
									</span>
								</div>
							</div>
						</div>

						<div className="flex justify-between rounded border border-(--line) bg-(--surface) p-2 text-[11px]">
							<span className="text-(--f4)">Integrated Sim Time:</span>
							<span className="text-(--f1)">
								{integrator?.time !== undefined && integrator.time > 0
									? integrator.time.toFixed(4)
									: "0.0000"}{" "}
								s
							</span>
						</div>
					</div>
				)}

				{tab === "gas" && (
					<div className="space-y-3 font-mono">
						<div className="grid grid-cols-2 gap-2">
							<div className="rounded border border-(--line) bg-(--surface) p-2">
								<div className="text-[9px] uppercase tracking-wider text-(--f4)">
									Max Mach Number
								</div>
								<div
									className={`text-base font-bold ${
										machVal > 1.0
											? "text-(--error)"
											: machVal > 0.7
												? "text-(--warning)"
												: "text-(--success)"
									}`}
								>
									{machVal.toFixed(3)}
								</div>
								<div className="text-[9px] text-(--f4)">
									{machVal > 1.0
										? "SUPERSONIC SHOCK"
										: machVal > 0.7
											? "TRANSONIC REGIME"
											: "SUBSONIC (STABLE)"}
								</div>
							</div>
							<div className="rounded border border-(--line) bg-(--surface) p-2">
								<div className="text-[9px] uppercase tracking-wider text-(--f4)">
									Velocity vs Sound
								</div>
								<div className="text-sm font-semibold text-(--f1)">
									{formatSci(gas?.maxSpeed, 3)} / {formatSci(gas?.maxSound, 3)}
								</div>
								<div className="text-[9px] text-(--f4)">v_max / c_sound</div>
							</div>
						</div>

						<div className="rounded border border-(--line) bg-(--surface) p-2.5">
							<div className="mb-1.5 text-[10px] font-medium uppercase tracking-wider text-(--f3)">
								Turbulence & Hydrodynamic Kinematics
							</div>
							<div className="grid grid-cols-2 gap-x-4 gap-y-1 text-[11px]">
								<div className="flex justify-between">
									<span className="text-(--f4)">Vorticity RMS:</span>
									<span className="text-(--f2)">{formatSci(vorticityVal, 4)}</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Vorticity Max:</span>
									<span className="text-(--f2)">
										{formatSci(gas?.vorticityMax, 4)}
									</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Strain RMS:</span>
									<span className="text-(--f2)">{formatSci(strainVal, 4)}</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Strain Max:</span>
									<span className="text-(--f2)">
										{formatSci(gas?.strainMax, 4)}
									</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Viscous Power:</span>
									<span className="text-(--f2)">
										{formatSci(gas?.viscousPower, 5)}
									</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Viscosity Proxy η:</span>
									<span className="text-(--f2)">
										{formatSci(viscosityProxy, 4)}
									</span>
								</div>
							</div>
						</div>

						<div className="rounded border border-(--line) bg-(--surface) p-2.5">
							<div className="mb-1.5 text-[10px] font-medium uppercase tracking-wider text-(--f3)">
								Gas Energy Partition & State Bounds
							</div>
							<div className="grid grid-cols-2 gap-x-4 gap-y-1 text-[11px]">
								<div className="flex justify-between">
									<span className="text-(--f4)">E_internal:</span>
									<span className="text-(--f2)">{formatSci(gasInternal, 3)}</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">E_kinetic:</span>
									<span className="text-(--f2)">{formatSci(gasKinetic, 3)}</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Min Density ρ:</span>
									<span
										className={
											(gas?.minDensity ?? 0) <= 0
												? "text-(--error)"
												: "text-(--f2)"
										}
									>
										{formatSci(gas?.minDensity, 4)}
									</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Min Pressure P:</span>
									<span
										className={
											(gas?.minPressure ?? 0) <= 0
												? "text-(--error)"
												: "text-(--f2)"
										}
									>
										{formatSci(gas?.minPressure, 4)}
									</span>
								</div>
							</div>
						</div>
					</div>
				)}

				{tab === "wave" && (
					<div className="space-y-3 font-mono">
						<div className="grid grid-cols-2 gap-2">
							<div className="rounded border border-(--line) bg-(--surface) p-2">
								<div className="text-[9px] uppercase tracking-wider text-(--f4)">
									Wave Norm ‖Ψ‖
								</div>
								<div className="text-base font-bold text-info">
									{formatSci(waveNorm, 4)}
								</div>
								<div className="text-[9px] text-(--f4)">
									Projected: {formatSci(wave?.projectedNorm, 4)}
								</div>
							</div>
							<div className="rounded border border-(--line) bg-(--surface) p-2">
								<div className="text-[9px] uppercase tracking-wider text-(--f4)">
									Kuramoto Order R
								</div>
								<div className="text-base font-bold text-(--acc)">
									{formatSci(kuramotoR, 4)}
								</div>
								<div className="text-[9px] text-(--f4)">
									Phase Synchrony (0-1)
								</div>
							</div>
						</div>

						<div className="rounded border border-(--line) bg-(--surface) p-2.5">
							<div className="mb-1.5 text-[10px] font-medium uppercase tracking-wider text-(--f3)">
								Quantum Guidance & Wave Mechanics
							</div>
							<div className="grid grid-cols-2 gap-x-4 gap-y-1 text-[11px]">
								<div className="flex justify-between">
									<span className="text-(--f4)">Guidance Speed v_B:</span>
									<span className="text-(--f2)">{formatSci(guidanceSpeed, 4)}</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Coherence ⟨|Ψ|²⟩:</span>
									<span className="text-(--f2)">{formatSci(coherenceMag2, 4)}</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Wave Kinetic:</span>
									<span className="text-(--f2)">{formatSci(wave?.kinetic, 4)}</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Wave Potential:</span>
									<span className="text-(--f2)">{formatSci(wave?.potential, 4)}</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Chemical Pot μ:</span>
									<span className="text-(--f2)">{formatSci(wave?.chemical, 4)}</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Phase Potential:</span>
									<span className="text-(--f2)">
										{formatSci(wave?.phasePotential, 4)}
									</span>
								</div>
							</div>
						</div>

						<div className="rounded border border-(--line) bg-(--surface) p-2.5">
							<div className="mb-1.5 text-[10px] font-medium uppercase tracking-wider text-(--f3)">
								Pilot Wave Coupling
							</div>
							<div className="grid grid-cols-2 gap-x-4 gap-y-1 text-[11px]">
								<div className="flex justify-between">
									<span className="text-(--f4)">Speed RMS:</span>
									<span className="text-(--f2)">{formatSci(pilot?.speedRms, 4)}</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Displacement RMS:</span>
									<span className="text-(--f2)">
										{formatSci(pilot?.displacementRms, 4)}
									</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Density Median:</span>
									<span className="text-(--f2)">
										{formatSci(pilot?.densityMedian, 4)}
									</span>
								</div>
								<div className="flex justify-between">
									<span className="text-(--f4)">Integration Error:</span>
									<span className="text-(--f2)">
										{formatSci(pilot?.integrationErrorMax, 5)}
									</span>
								</div>
							</div>
						</div>
					</div>
				)}

				{tab === "residuals" && (
					<div className="space-y-3 font-mono">
						<div className="rounded border border-(--line) bg-(--surface) p-2.5">
							<div className="mb-1.5 text-[10px] font-medium uppercase tracking-wider text-(--f3)">
								Conservation Residuals & Balances
							</div>
							<div className="space-y-2 text-[11px]">
								<div className="flex justify-between items-center border-b border-(--line) pb-1">
									<span className="text-(--f4)">Gas Energy Residual:</span>
									<span
										className={`font-semibold ${
											Math.abs(sources?.gasEnergyResidual ?? 0) > 1e-3
												? "text-(--error)"
												: "text-(--success)"
										}`}
									>
										{formatSci(sources?.gasEnergyResidual, 6)}
									</span>
								</div>
								<div className="flex justify-between items-center border-b border-(--line) pb-1">
									<span className="text-(--f4)">Conservative Wave Error:</span>
									<span
										className={`font-semibold ${
											Math.abs(sources?.conservativeWaveError ?? 0) > 1e-3
												? "text-(--error)"
												: "text-(--success)"
										}`}
									>
										{formatSci(sources?.conservativeWaveError, 6)}
									</span>
								</div>
								<div className="flex justify-between items-center border-b border-(--line) pb-1">
									<span className="text-(--f4)">PIC Deposit Residual:</span>
									<span className="text-(--f2)">
										{formatSci(sources?.picDepositEnergyResidual, 6)}
									</span>
								</div>
								<div className="flex justify-between items-center border-b border-(--line) pb-1">
									<span className="text-(--f4)">Particle Balance Residual:</span>
									<span className="text-(--f2)">
										{formatSci(sources?.particleBalanceResidual, 6)}
									</span>
								</div>
								<div className="flex justify-between items-center">
									<span className="text-(--f4)">Gravity Balance Residual:</span>
									<span className="text-(--f2)">
										{formatSci(sources?.gravityBalanceResidual, 6)}
									</span>
								</div>
							</div>
						</div>

						<div className="rounded border border-(--line) bg-(--surface) p-2.5 text-[11px]">
							<div className="mb-1 text-[10px] font-medium uppercase tracking-wider text-(--f3)">
								Divergence & Boundary Checks
							</div>
							<div className="flex justify-between py-0.5">
								<span className="text-(--f4)">Velocity Divergence ∇·u:</span>
								<span className="text-(--f1)">{formatSci(divergence, 5)}</span>
							</div>
							<div className="flex justify-between py-0.5">
								<span className="text-(--f4)">Particle Material Total:</span>
								<span className="text-(--f1)">
									{formatSci(health?.particleMaterialTotal, 4)}
								</span>
							</div>
						</div>
					</div>
				)}
			</div>
		</div>
	);
};
