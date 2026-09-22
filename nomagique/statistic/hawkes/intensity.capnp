@0xd0e91bcd2fc82fb8;

using Go = import "/go.capnp";
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

# Intensity evaluates the conditional intensity of each component at the
# horizon: lambda_k = baseline_k + sum over j of excitation[k][j] * support_j.
interface Intensity {
  write @0 (
    baseline   :List(Float64),
    excitation :List(Float64),
    support    :List(Float64),
    dimension  :Int32,
  ) -> stream;

  done @1 () -> (
    intensity :List(Float64),
  );
}
