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
  write @0 (value :Float64, epoch :Int64, sequence :Int64, scope :Text) -> stream;
  done @1 () -> ExcursionResult;
}

struct ExcursionResult {
 union {
  none @0 :Void;
  move :group {
   anchor @1 :Float64;
   ignition @2 :Float64;
   extremum @3 :Float64;
   excursion @4 :Float64;
   confirmed @5 :Bool;
   anchorSequence @12 :Int64;
   ignitionSequence @13 :Int64;
   extremumSequence @14 :Int64;
   confirmationSequence @15 :Int64;
   row @16 :Data;
   hasPrecursor @19 :Bool;
  }
 }
 qualifying @6 :Float64;
 sigma @7 :Float64;
 floor @8 :Float64;
 horizon @9 :Float64;
 steps @10 :Int64;
 legs @11 :Int64;
 epoch @17 :Int64;
 scope @18 :Text;
}
