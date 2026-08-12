import { Component } from "#/components/ui/component";
import { RESONANCE_FOCUS } from "#/providers/ws-stores";
/*
The resonance batch carries every settled carrier, not just the focused one, so
this reads the focused-carrier stream rather than a position in that batch.
*/

export const LiveResonanceTitle = () => (
  <Component registerKey={RESONANCE_FOCUS}>
    {({ ref, className }) => (
      <span ref={ref} className={className}>
        h
        <span data-paint="forecast.supportedHorizon" data-paint-format=".0f">
          —
        </span>
        {" · r "}
        <span data-paint="forecast.nextReach" data-paint-format=".0f">
          —
        </span>
        {" · precision "}
        <span data-paint="taskPrecision" data-paint-format=".3f">
          —
        </span>
      </span>
    )}
  </Component>
);
