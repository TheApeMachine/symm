@0xb2990383054052bc;

using Go = import "/go.capnp";
$Go.package("temporal");
$Go.import("github.com/theapemachine/symm/nomagique/temporal");

# scope names the series being read. Readings under a new scope start a new
# history: a series never carries state into another one. An unwired scope is
# one series for the node's lifetime.
#
# Elapsed is the time in seconds between the timestamp just read, in
# nanoseconds since the epoch, and the one before it. With origin set it is measured from the first reading of the
# series instead: how long the series has run, the span a cumulative total is
# spread over.
interface Elapsed {
  write @0 (timestamp :Float64, scope :Text, origin :Bool) -> stream;
  done @1 () -> (out :Float64);
}
