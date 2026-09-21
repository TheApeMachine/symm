using Go = import "/go.capnp";
@0xc8e04b1a9d3f6275;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

using import "../runtime/status.capnp".Status;

# Transform is a function a graph hands to another node. Anything implementing
# it can be wired into a body port and called back per element.
interface Transform {
  apply @0 (value :Float64) -> (out :Float64);
}

# Scale is the simplest Transform: it multiplies by a factor. It exists so a
# graph has a function to wire into a body port, and so anything else
# implementing Transform is wired the same way.
interface Scale extends(Transform) {
  write @0 (factor :Float64) -> stream;
  done  @1 () -> (factor :Float64);
}

# Map applies a Transform to every element of the collection at a path, and
# emits the structure carrying the transformed collection. The body runs once
# per element within one observation, so a collection is mapped whole rather
# than across evaluations.
interface Map {
  write @0 (data :Data, path :Text, body :Transform) -> stream;
  done @1 () -> (out :Data, count :Int64, status :Status);
}
