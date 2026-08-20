# `logic/manifold` — the market as a resident field

The manifold solver maps the complete subscribed market universe into one
resident Metal simulation. Symbols share the same gas and wave fields so their
interference remains observable.

## Active data path

```text
Kraken L3 books
      │
      ▼
market oscillators ──► physics/manifold ──► resident Metal buffers
                                                │
                   ┌────────────────────────────┼─────────────────────┐
                   ▼                            ▼                     ▼
          SFF1 field slab              SPF1 particle slab      FlatBuffer phase
          momRho/eInt/ψ                 interleaved float32     semantic state
                   │                            │                     │
                   └──────────── ordered, reliable WebRTC ───────────┘
                                                │
                                                ▼
                                      interactive browser 3D GPU
```

There is no backend-rendered RGBA display in this path. The backend publishes
the simulation state, and the browser retains camera control, depth, slices,
visibility controls, and particle picking.

## Wire responsibilities

`SFF1` and `SPF1` are raw numerical slab protocols. They deliberately do not
use FlatBuffers because their payload is already in the layouts consumed by
browser GPU resources.

- `SFF1` has a fixed 64-byte header followed by Metal's native
  `momentum.xyz/density.w`, internal-energy, wave-real, and wave-imaginary
  float32 arrays.
- `SPF1` has a fixed 64-byte header followed by one interleaved particle array:
  position, velocity, mass, heat, energy, phase, omega, and amplitude.
- `fluid-phase` remains a schema-tagged FlatBuffer because it contains rich
  semantic state: readiness, hydrodynamics, wave modes, and phase-scan results.

The WebRTC `SFD1` record layer only segments and reassembles these payloads for
SCTP. It does not reinterpret their contents.

## Browser ownership

The frontend constructs typed views over the completed record buffer. It does
not rebuild field arrays or allocate one object per particle. Field views are
bound to 3D textures, and the interleaved particle view is bound to one GPU
buffer. A particle object is materialized only when picking requests a readout.

GPU upload is still required because WebRTC receive memory is not browser GPU
memory. The protocol removes avoidable CPU reshaping and copying around that
required upload.

## Phase dial

Each symbol contributes a carrier to the shared wave field. The solver retains
settled fingerprints with their realized forward price outcomes and scans
phase rotations against that corpus. The labels are observed price outcomes,
not another model's classification. The current cut is excluded from its own
scan.

Phase state is observational today: it is published and rendered but does not
change trading decisions.

## Files

| File | Responsibility |
|---|---|
| `solver.go` | Owns the resident simulation and publishes settled state. |
| `slab.go` | Encodes GPU-shaped `SFF1` and `SPF1` numerical slabs. |
| `phase.go` | Retains outcomes and performs the angular phase sweep. |
| `constants.go` | Defines documented domain bounds. |

Metal shader changes in `nomagique/physics/manifold/manifold.metal` require the
package's metallib generation step before Go embeds the updated library.
