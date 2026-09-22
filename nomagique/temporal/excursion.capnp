@0xd41f7a25b6c8e930;

using Go = import "/go.capnp";
$Go.package("temporal");
$Go.import("github.com/theapemachine/symm/nomagique/temporal");

# Excursion finds the moves a path actually made.
#
# A path is walked as alternating legs. A leg runs until the path pulls back
# further than the walk would by chance over the leg's own length, so a leg
# closes on evidence that the move is over rather than on the first step
# against it. A leg that travelled further than this path's own moves
# typically do qualifies.
#
# Nothing here is declared. The path says how far is far: the bar is the mean
# and dispersion of the moves it has already made, floored by the smallest
# change it is able to express at all, and bootstrapped before any move has
# closed by what a walk of the same dispersion would reach over the path's own
# characteristic span. The span is measured too — it is how long this path's
# moves run.
# Bounds use log returns. The reversal scale is sigma * sqrt(open-leg steps).
# The qualifying scale is the RMS log magnitude of completed legs; before
# any close it is sigma * sqrt(mean directional-run length). RMS combines
# the observed mean and variance without a confidence multiplier. The floor
# is the minimum observed nonzero absolute log return. A move must exceed
# its bound strictly. The excursion output remains the relative price change.
interface Excursion {
  write @0 (value :Float64) -> stream;
  done @1 () -> (
    anchor     :Float64,
    ignition   :Float64,
    extremum   :Float64,
    excursion  :Float64,
    qualifying :Float64,
    sigma      :Float64,
    floor      :Float64,
    horizon    :Float64,
    steps      :Int64,
    legs       :Int64,
    confirmed  :Bool,
    found      :Bool
  );
}
