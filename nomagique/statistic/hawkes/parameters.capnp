@0xb341aea5cd43d38f;

using Go = import "/go.capnp";
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

# Parameters maps a point in the optimizer's unconstrained coordinates onto
# the parameters of the process, squashing each coordinate into its bound
# through a softplus ratio and exponentiating out of log space. It also
# reports the derivative of that map, so a gradient measured in natural
# parameters can be carried back into the coordinates the optimizer moves in.
#
# Coordinates are ordered exactly as LogLikelihood reports its gradient: the
# baselines, then the excitation matrix row-major, then the decay rate. Each
# one is its own parameter in log space, so the map is diagonal and a
# gradient measured in natural parameters is carried back by multiplying it
# entry by entry with the jacobian.
interface Parameters {
  write @0 (
    coordinates :List(Float64),
    lower       :List(Float64),
    upper       :List(Float64),
    dimension   :Int32,
  ) -> stream;

  done @1 () -> (
    baseline   :List(Float64),
    excitation :List(Float64),
    decay      :Float64,
    jacobian   :List(Float64),
  );
}
