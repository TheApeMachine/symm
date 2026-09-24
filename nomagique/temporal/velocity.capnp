@0xe7d74680109a8b99;

using Go = import "/go.capnp";
$Go.package("temporal");
$Go.import("github.com/theapemachine/symm/nomagique/temporal");

# Velocity is how fast an observable is moving, fitted across the observations
# it has seen rather than read off the last two.
#
# The slope travels with the confidence behind it: a rate whose uncertainty is
# as large as itself is not evidence of a move, and two points can never say
# which of the two it is.
# scope names the series being read. Readings under a new scope start a new
# history: a series never carries state into another one. An unwired scope is
# one series for the node's lifetime.
interface Velocity {
  write @0 (val :Float64, ts :Float64, scope :Text) -> stream;
  done @1 () -> (
    out     :Float64,
    snr     :Float64,
    defined :Bool
  );
}
