# Component Direct Paint

## Overview

The direct-paint system registers one `Component` paint callback per backend wire key in `frontend/src/providers/ws-stores.ts`. Each callback receives `paint(updates)` payloads and applies updates only within the DOM subtree owned by that `Component` instance.

`frontend/src/components/ui/component.tsx` provides the local rendering scope by exposing a root ref and scanning only that subtree for supported paint attributes.

## Component paint registration contract

- A `Component` registers a paint callback under one backend key such as `measurements`, `decisions`, `tick`, or `cortex`.
- `paintRegistered(key, updates)` dispatches frame entries only to callbacks registered for that key.
- Each component instance updates its own scoped DOM subtree; no global selector pass is used for direct paint.

## Supported data attributes

The current implementation supports these active attributes:

- `data-paint`: reads a value from the `paint(updates)` payload and writes text content by default.
- `data-paint-format`: formats numeric values before painting, as implemented by `component.tsx`.
- `data-paint-class`: toggles classes based on expected-value rules.
- `data-paint-prop`: writes a painted value to a DOM property path instead of `textContent`.
- `data-set`: reads a value from the payload and applies it to a property path named by `data-target`.
- `data-target`: property path used together with `data-set`.
- `data-scope`, `data-filter`, and `data-index`: narrow array payloads to one matching row before paint application.

These attributes align directly with payload keys inside `paint(updates)`. For example, `data-paint="source"` reads the `source` key from the selected payload object, while `data-set="bar_width" data-target="style.width"` applies the `bar_width` payload value to `style.width`.

## Planned or unsupported attributes

The following attributes are not part of the active contract in `component.tsx` and should be treated as inert or planned unless explicit implementation is added:

- `data-append`
- `data-transform`
- `data-update`

Existing emitting components such as `frontend/src/components/terminal/kernel-list.tsx` and `frontend/src/components/dashboard/kernels.tsx` should use the implemented `data-paint`, `data-paint-format`, `data-paint-class`, `data-set`, and `data-target` contract.

## Payload shapes and retained data

The direct-paint system accepts both flat objects and arrays of typed rows:

- Flat payloads are painted directly against matching `data-paint` or `data-set` keys.
- Array payloads are narrowed by `data-scope`/`data-filter` or `data-index` before field reads occur.
- Nested keys are resolved through dotted paths such as `validity.state`.

This model is retained-data oriented. Incoming updates modify only the smallest affected DOM targets and do not rebuild the subtree. Canvas-based renderers may retain their own drawing state, while DOM-based direct paint retains the last written values in-place until a later update changes them.