import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { IntegratorHealthT } from "#/providers/telemetry/telemetry/integrator-health";
import { PhysicsHealthT } from "#/providers/telemetry/telemetry/physics-health";
import { PhysicsDiagnosticsHUD } from "./physics-diagnostics";
import type { FluidPhaseReading } from "./wire";

const render = (phaseReading: FluidPhaseReading | null) => renderToStaticMarkup(
 <PhysicsDiagnosticsHUD isOpen onClose={() => {}} phaseReading={phaseReading} particleCount={2} />,
);

describe("PhysicsDiagnosticsHUD", () => {
 it("shows waiting until accepted physics health arrives", () => {
  expect(render(null)).toContain("WAITING");
  expect(render(null)).not.toContain("ACCEPTED");
 });
 it("renders accepted time and numerical rejections from the received health", () => {
  const integrator = new IntegratorHealthT();
  integrator.acceptedDt = 0.125;
  integrator.time = 1.25;
  integrator.substeps = 2;
  const reading: FluidPhaseReading = {
   divergence: 0, guidanceSpeed: 0, coherenceMag2: 0, pressureGradNorm: 0,
   viscosityProxy: 0, kuramotoR: 0, kuramotoPsi: 0,
   health: new PhysicsHealthT(integrator),
  };
  expect(render(reading)).toContain("ACCEPTED");
  expect(render(reading)).toContain("0.125");
  expect(render(reading)).toContain("unbounded");
  integrator.rejections = 1;
  expect(render(reading)).toContain("1 REJECTIONS");
  expect(render(reading)).not.toContain("ACCEPTED");
 });
});
