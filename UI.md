# SYMM Compiled UI Graph

## Objective

Replace the current partially hard-coded UI graph/layout model with a compiler-generated system derived directly from the existing React UI component library.

The UI graph must describe component composition, configuration, and routing. It must **not** introduce a second layout system, a parallel component catalog, or UI-specific runtime machinery that duplicates capabilities already present in React, CSS/Tailwind, TanStack Router, or Flume.

The intended end state is:

1. The existing React component library is the authoritative component catalog.
2. Component prop types are reflected during generation/compilation.
3. The compiler generates the exact Flume inputs required for each component.
4. The compiler generates a component registry mapping component names to React component function references.
5. UI route graphs are rendered by recursively resolving nodes through that registry.
6. Layout is expressed through existing structural components and normal `className` values.
7. TanStack Router routes are derived from the authored UI route graph rather than maintained separately.

The goal is to eliminate the recurring manual work required to reconnect frontend UI structure after backend/compiler changes.

---

## Architectural principles

### One source of truth

Do not maintain a second hand-written catalog of frontend components.

The source of truth is the exported React UI component library, currently exposed through:

`frontend/src/components/ui/index.ts`

The generated system must derive its component metadata and registry from that surface.

A component added to the UI library should become available to the UI graph by regeneration, without requiring a manually maintained Cap'n Proto enum or switch statement.

---

## Remove the existing layout abstraction

Delete the UI-specific grid/span abstraction.

Specifically:

* Remove `nomagique/ui/section.capnp`.
* Remove `UISection`.
* Remove its Go server implementation and generated registration.
* Remove `Section` structures from `route.capnp`.
* Remove any top/right/bottom/left span semantics created specifically for the graph editor.

Do **not** replace it with another custom grid representation.

Layout already belongs to the component library and CSS.

Examples of existing structural components include:

* `Flex`
* `Grid`
* `Section`
* `Panel`
* `Frame`
* `Rail`
* `Nav`
* `Toolbar`
* ordinary HTML-compatible components

Graph-authored layout should use these components just like hand-written React would.

Example:

```text
Flex.Column
  className="h-full min-h-0 gap-4 p-4"

  Panel
    className="min-h-0 flex-1"

  Flex.Row
    className="grid grid-cols-3 gap-3"
```

No positional node coordinates or span data should affect rendered layout.

Flume node position remains editor metadata only.

---

## Remove the hard-coded component type catalog

The current `ComponentType` enum in `component.capnp` is not the authoritative component catalog and should not remain one.

Do not keep a manually maintained enum such as:

```text
alert
badge
button
callout
...
```

That catalog will drift from the actual component library.

The component identity stored in the UI graph should instead be a stable component name corresponding to an entry in the generated component registry.

Conceptually:

```text
component: "Panel"
component: "Flex.Column"
component: "Badge"
component: "Meter"
```

Exact encoding is an implementation detail, but adding a frontend component must not require updating a central enum by hand.

---

## Component reflection

Use the TypeScript compiler/type checker to inspect the actual exported React component types.

Do not implement this as a regex scanner or syntax-only parser.

This is important because the component library uses types such as:

* `ComponentProps<"div">`
* `ComponentPropsWithoutRef<...>`
* aliases
* intersections
* `VariantProps<typeof ...>`
* literal unions
* inherited `className`
* optional props

The generator should resolve the effective TypeScript type for each component.

Produce a normalized component metadata representation.

Conceptually:

```ts
{
  name: "Panel",
  props: {
    variant: {
      kind: "enum",
      values: ["default", "sunken", "raised"],
      optional: true
    },
    size: {
      kind: "enum",
      values: ["sm", "md", "lg"],
      optional: true
    },
    className: {
      kind: "string",
      optional: true
    },
    children: {
      kind: "children",
      optional: true
    }
  }
}
```

The exact metadata format may differ, but it must be generated rather than maintained manually.

---

## Prop filtering

Do not expose every property inherited from React DOM types as a Flume input.

Reflect the real type first, then deliberately select graph-configurable properties.

Expose:

* component-specific props
* literal-union variant props
* strings
* numbers
* booleans
* appropriate serializable arrays/records where useful
* `className`
* selected useful native properties such as `href`, `title`, `disabled`, `placeholder`, etc.
* structural `children`

Do not expose implementation-only React properties such as:

* `ref`
* event callback functions by default
* internal framework properties
* arbitrary function-valued props
* properties that cannot be represented declaratively in the graph

If a property cannot be serialized/configured meaningfully, omit it instead of inventing an encoding.

---

## Flume generation

Extend the existing compiler generation path that creates:

`frontend/src/components/flume/flume-config.generated.ts`

The generator already supports dynamic inputs for nodes such as `controlflow.Batch` and reusable definitions.

Use the same mechanism for UI component nodes.

For each reflected component, generate a Flume node whose inputs match that component's configurable props.

Map normalized types to controls mechanically.

Expected mappings include:

```text
string              -> text input
multiline string    -> textarea where appropriate
number              -> number input
boolean             -> checkbox
literal union       -> select
enum-like union     -> select
children            -> graph structural connection
className           -> text input
```

Do not hard-code individual component layouts into the Flume generator except where a genuine exceptional component requires it.

The normal case must be metadata-driven.

---

## Structural children

React `children` represents graph structure, not a normal scalar input control.

Parent-child relationships in the authored UI graph must determine rendered nesting.

For example:

```text
Route
  -> Flex.Column
       -> Panel
            -> Meter
       -> Panel
            -> Badge
```

must render approximately as:

```tsx
<Flex.Column>
  <Panel>
    <Meter />
  </Panel>
  <Panel>
    <Badge />
  </Panel>
</Flex.Column>
```

Do not serialize rendered JSX or HTML into graph values.

The graph remains structural.

---

## Generated component registry

Generate a frontend component registry at compile/generation time.

Conceptually:

```ts
export const uiComponents = {
  Alert,
  Badge,
  Button,
  Callout,
  Canvas,
  Card,
  Checkbox,
  Flex,
  Frame,
  Grid,
  Meter,
  Panel,
  Section,
  ...
}
```

Nested exports such as `Flex.Row`, `Flex.Column`, `Panel.Header`, etc. should be represented consistently if they are part of the usable public UI surface.

The registry key must correspond to the component identity stored in UI graph nodes.

Rendering must not require a large hand-written switch:

```ts
switch (node.type) {
  case "Panel":
  case "Badge":
  ...
}
```

Instead:

```ts
const Component = uiComponents[node.component]
```

or an equivalent generated lookup.

Unknown component names should fail clearly during compilation or graph validation rather than silently rendering nothing.

---

## Runtime renderer

Implement a small generic renderer for a compiled UI subgraph.

Its responsibility is approximately:

1. Resolve the node's component name through the generated registry.
2. Resolve configured prop values.
3. Find structural child nodes.
4. Render those children recursively.
5. Pass configured props and children to the component.

Conceptually:

```tsx
function renderNode(node: CompiledUINode): ReactNode {
  const Component = uiComponents[node.component]

  return (
    <Component {...node.props}>
      {node.children.map(renderNode)}
    </Component>
  )
}
```

The real implementation may require keys, data bindings, route context, and error handling, but it should remain this conceptually simple.

Do not create component-specific renderer branches unless genuinely unavoidable.

---

## Routing

Use TanStack Router's programmatic/virtual route facilities rather than maintaining a parallel set of manually authored route files for graph-defined pages.

Reference:

`https://tanstack.com/router/latest/docs/routing/virtual-file-routes`

A UI route should minimally describe things such as:

```text
path
title/metadata if needed
root UI node/subgraph
parent route/layout relationship if applicable
```

Do not model routes as lists of artificial `Section` objects.

The route points at a UI component subgraph.

Conceptually:

```text
Route "/cortex"
    |
    -> Flex.Column
         -> ...
```

Support nested/pathless layouts where they map naturally to TanStack Router.

### Build-time vs runtime consideration

TanStack virtual file routes are primarily a route-generation/build facility.

Choose the simplest approach consistent with SYMM's intended workflow:

* If UI graph compilation already causes frontend regeneration/build, generate the actual TanStack virtual route configuration.
* If routes must become available dynamically without rebuilding the frontend bundle, generate a stable TanStack route shell whose route component loads/renders the compiled UI graph.

Do not introduce both approaches unless there is a demonstrated need.

---

## `className`

`className` is the primary escape hatch for authored layout and styling.

The UI graph must permit configuring it directly where the component type supports it.

This allows nodes to use normal CSS/Tailwind composition:

```text
className="grid grid-cols-3 gap-4"
```

or:

```text
className="h-full min-h-0 flex-1 overflow-hidden"
```

Do not parse Tailwind classes into graph layout properties.

Do not introduce row/column/span abstractions merely to represent things CSS already expresses directly.

The browser/CSS engine remains the layout engine.

---

## Drag-and-drop

No runtime drag-and-drop page builder is required.

Flume can remain the graph authoring interface, but rendered UI layout is determined entirely by:

* graph parent/child relationships
* component props
* `className`
* existing structural components

Flume canvas coordinates are not part of UI runtime semantics.

Do not build resize handles, span editors, grid placement systems, or WYSIWYG layout logic unless separately requested later.

---

## Compiler authority

The compiler/generator must remain the authoritative bridge between backend schemas, frontend components, and Flume.

The desired generation pipeline is approximately:

```text
Cap'n Proto primitives ─────┐
                            ├─> compiler generation
React UI component types ───┘
                                  |
                                  ├─> factories_gen.go
                                  ├─> flume-config.generated.ts
                                  ├─> ui-component-registry.generated.ts
                                  └─> UI component metadata / route metadata
```

Do not add another manually synchronized metadata file.

Generated artifacts should clearly state that they are generated and must not be edited manually.

---

## UI Cap'n Proto scope

Keep Cap'n Proto UI primitives only where they represent meaningful graph concepts.

`UIRoute` may remain if routes are genuine graph-level nodes.

A generic UI component graph node may remain if useful, but it should reference generated component identity rather than contain a hand-maintained component enum.

Do not encode the frontend component library itself manually into `.capnp` files.

Do not represent structural frontend components such as Section/Grid/Flex as bespoke backend layout primitives.

---

## Validation

Compilation should reject:

* unknown component names
* props that do not exist on the selected component
* values incompatible with reflected prop types where statically checkable
* invalid structural relationships where relevant
* UI graphs without a valid render root
* duplicate/conflicting route definitions

Errors should identify the graph node and property involved so Flume can surface useful diagnostics.

---

## Tests

Add tests proving the integration rather than merely testing isolated helpers.

At minimum:

### Component discovery

Verify that exported components from the public UI library appear in generated component metadata/registry.

Choose several different component shapes, including:

* a simple component
* a CVA/variant component
* a component inheriting DOM props
* a structural component
* a compound/nested component if supported

### Prop reflection

Verify:

* `className` is exposed where supported
* literal unions become select-compatible metadata
* booleans become boolean controls
* numbers become numeric controls
* `children` becomes structural metadata
* `ref` and callback/event props are not exposed as ordinary controls

### Flume generation

Verify the generated Flume configuration produces the correct inputs for at least two components with different prop sets.

There should be no manually duplicated component prop catalog in the test fixture.

### Registry

Verify every graph-exposed component name resolves to an actual React component function/reference.

### Rendering

Given a small authored UI graph such as:

```text
Flex.Column
  Panel
    Badge
  Panel
    Button
```

verify the runtime renderer produces the corresponding nested React structure and configured props.

### Routes

Given two route graphs, verify the generated TanStack route model contains both paths and points each route at the correct compiled UI root.

### Drift prevention

Generation should fail CI if generated artifacts are stale relative to the component library or compiler output.

---

## Cleanup

As part of this work, remove obsolete code instead of leaving compatibility layers indefinitely.

Expected cleanup includes:

* `section.capnp`
* `UISectionServer`
* generated Section Cap'n Proto files if no longer generated
* Section-specific route fields
* hard-coded `ComponentType` enumeration if replaced by generated component identity
* any unused span/grid authoring code
* renderer switch statements superseded by the generated registry
* manually maintained UI component catalogs superseded by reflection

Do not keep an old architecture beside the new architecture unless required by a concrete migration constraint.

---

## Non-goals

This task does **not** include:

* building a WYSIWYG page builder
* implementing runtime drag-and-drop
* creating a new CSS/layout engine
* creating a custom grid/span language
* replacing the existing UI component library
* duplicating React's component model in Cap'n Proto
* exposing every possible HTML attribute in Flume
* creating adapters around every component manually
* maintaining two separate route definitions
* adding arbitrary component-specific rendering code

---

## Acceptance criteria

The work is complete when:

1. `section.capnp` and its custom span/layout abstraction are gone.
2. The React UI component library is the source of truth for graph-available components.
3. Component metadata is generated using actual TypeScript type information.
4. Flume inputs are generated from reflected component props.
5. `className` can be authored directly for components that support it.
6. Structural child relationships in the graph render as React children.
7. A generated component-name → React-function registry exists.
8. The runtime renderer is generic and registry-driven.
9. UI route graphs produce TanStack Router route configuration or feed a stable dynamic route shell.
10. No manual component enum/catalog needs updating when an eligible component is added.
11. No custom span/grid positioning system is required for rendered layout.
12. Tests exercise the complete generation → graph → render path.
13. Generated artifacts are checked for drift in CI.
14. The resulting implementation reduces, rather than increases, the number of places that must be kept synchronized.

## Guiding constraint

When deciding between introducing another abstraction and using an existing primitive, use the existing primitive.

React already provides component composition.

The UI library already provides structural components.

CSS/Tailwind already provides layout.

Flume already provides dynamic node inputs.

TanStack Router already provides route composition.

The compiler's job is to reflect these capabilities and connect them, not reproduce them.
