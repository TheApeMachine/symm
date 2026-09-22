@0x9e5c489998076dc3;

using Go = import "/go.capnp";
$Go.package("optimization");
$Go.import("github.com/theapemachine/symm/nomagique/optimization");

# Objective presents a measured quantity to a minimizer.
#
# Two things usually stand between a model's own gradient and the one a
# search can step along. The model may be something to maximize while the
# search only descends, and the model's parameters may not be the
# coordinates the search moves in. Sense flips the first; the jacobian of a
# diagonal coordinate map carries the second.
#
# A sense of -1 turns a quantity to maximize into one to minimize. An empty
# jacobian leaves the gradient in the coordinates it arrived in.
interface Objective {
  write @0 (
    value    :Float64,
    gradient :List(Float64),
    jacobian :List(Float64),
    sense    :Float64,
  ) -> stream;

  done @1 () -> (
    fVal     :Float64,
    gradient :List(Float64),
  );
}
