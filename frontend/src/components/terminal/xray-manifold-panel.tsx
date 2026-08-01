import { cn } from "#/lib/utils";
import { Component } from "#/components/ui/component";

/*
XrayManifoldPanel is the static manifold reading shell. DRAW paints via
paintXrayManifold and paintXrayManifoldMeasurements.
*/
export const XrayManifoldPanel = () => (
	<Component registerKey="">
		{({ ref, className }) => (
	<div ref={ref} className={cn("flex flex-col gap-2 border-(--line) border-t px-3.5 py-3", className)}>
		<div>
			<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
				Manifold reading
			</div>
			<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
				|ψ|² · guidance current · particles
			</div>
		</div>
		<div className="grid grid-cols-2 gap-x-4 gap-y-2 font-mono text-[11px]">
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">∇·u</span>
				<span className="text-right text-(--f1)" />
			</div>
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">|ψ|²</span>
				<span className="text-right text-(--f1)" />
			</div>
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">guide v</span>
				<span className="text-right text-(--f1)" />
			</div>
			<div className="flex justify-between gap-3">
				<span className="text-(--f3)">viscosity</span>
				<span className="text-right text-(--f1)" />
			</div>
		</div>
		<div className="mt-0.5">
			<div className="mb-1 flex justify-between text-[10px]">
				<span className="text-(--f3)">momentum eigenmode</span>
				<span className="font-mono" />
			</div>
			<div className="relative">
				<div className="h-1.5 overflow-hidden rounded-[3px] bg-(--line)">
					<div className="h-full" style={{ width: "0%" }} />
				</div>
				<div className="relative h-0">
					<div className="absolute -top-2.25 left-[40%] h-3 w-0.5 bg-(--acc)" />
				</div>
			</div>
			<div className="mt-1.5 font-mono text-[8.5px] text-(--f4)">
				drive playbook gate · mode share ≥ 0.40
			</div>
		</div>
	</div>
		)}
	</Component>
);
