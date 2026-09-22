@0xaadd5c33d68af8e0;

using Go = import "/go.capnp";
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

# KernelIntegral evaluates the closed-form integral of the exponential kernel
# over the observation window (origin, horizon] for each component, together
# with its derivative with respect to the decay rate. The integral is what
# the compensator is built from; the derivative is what lets the likelihood
# report an analytic gradient in the decay direction.
interface KernelIntegral {
  write @0 (
    times      :List(Float64),
    components :List(Float64),
    origin     :Float64,
    horizon    :Float64,
    decay      :Float64,
    dimension  :Int32,
  ) -> stream;

  done @1 () -> (
    support         :List(Float64),
    decayDerivative :List(Float64),
  );
}
