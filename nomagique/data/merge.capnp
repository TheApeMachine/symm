using Go = import "/go.capnp";
@0xaa107350cc7adf60;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

# Overlays named fields; both operands must be explicit JSON objects.
interface Merge {
  write @0 (base :Data, overlay :Data) -> stream;
  done @1 () -> (out :Data);
}
