import artifactCapnp from "#/lib/capnp/artifact.capnp.js";

export const Artifact = artifactCapnp.Artifact;

export type ArtifactRoot = InstanceType<typeof Artifact>;
