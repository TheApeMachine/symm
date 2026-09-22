@0x9e6491875dbd7840;

using Go = import "/go.capnp";
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

# StationaryIntensity solves (identity - branching) * intensity = baseline,
# the long-run mean intensity of each component once every generation of
# excitation is accounted for. It is defined only for a subcritical process.
interface StationaryIntensity {
  write @0 (
    baseline  :List(Float64),
    branching :List(Float64),
    dimension :Int32,
  ) -> stream;

  done @1 () -> (
    intensity :List(Float64),
    defined   :Bool,
  );
}
