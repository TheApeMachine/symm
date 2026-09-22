@0xe4b266ca57c12629;

using Go = import "/go.capnp";
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

# LogLikelihood evaluates the exact log-likelihood of the observed window
# under one parameter set, plus its analytic gradient. The gradient is laid
# out in the natural parameter order the rest of this package uses: the
# baselines, then the excitation matrix row-major, then the decay rate.
#
# The integrated kernel it subtracts is not recomputed here: KernelIntegral
# already owns that quantity and its decay derivative, so both arrive as
# ports and this node only assembles them.
#
# It reports whether the value is defined rather than substituting one: a
# parameter set that drives any intensity non-positive has no likelihood, and
# saying so is what lets an optimizer reject the candidate.
interface LogLikelihood {
  write @0 (
    times      :List(Float64),
    components :List(Float64),
    origin     :Float64,
    horizon    :Float64,
    baseline   :List(Float64),
    excitation :List(Float64),
    decay      :Float64,
    dimension  :Int32,
    integral   :List(Float64),
    integralDecayDerivative :List(Float64),
  ) -> stream;

  done @1 () -> (
    value    :Float64,
    gradient :List(Float64),
    defined  :Bool,
  );
}
