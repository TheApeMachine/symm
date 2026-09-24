@0xcf9c1fe749579fb8;

using Go = import "/go.capnp";
$Go.package("temporal");
$Go.import("github.com/theapemachine/symm/nomagique/temporal");

# scope names the series being read. Readings under a new scope start a new
# history: a series never carries state into another one. An unwired scope is
# one series for the node's lifetime.
interface Delay {
  write @0 (value :Float64, scope :Text) -> stream;
  done @1 () -> (out :Float64);
}
