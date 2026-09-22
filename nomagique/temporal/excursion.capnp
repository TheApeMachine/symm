@0xd41f7a25b6c8e930;

using Go = import "/go.capnp";
$Go.package("temporal");
$Go.import("github.com/theapemachine/symm/nomagique/temporal");

# Excursion finds the moves a path actually made.
#
# A path is walked as alternating legs. A leg runs until the path retraces a
# proportion of what that leg travelled, so a leg closes on evidence that the
# move is over rather than on the first step against it. A leg that travelled
# far enough qualifies.
#
# Far enough is derived, never declared: it is sigmas of the path's own
# per-step log return, scaled over horizon steps the way a random walk scales,
# and floored so a numerically dead path cannot manufacture moves out of
# quantisation noise.
#
# What comes back is the three points a qualified move is made of. Anchor is
# where the previous leg began, ignition is where this leg began, and extremum
# is the furthest it reached. A run into ignition is what a precursor looks
# like; the run out of it is what the move was worth.
interface Excursion {
  write @0 (
    value   :Float64,
    sigmas  :Float64,
    horizon :Int32,
    retrace :Float64,
    floor   :Float64
  ) -> stream;
  done @1 () -> (
    anchor     :Float64,
    ignition   :Float64,
    extremum   :Float64,
    excursion  :Float64,
    qualifying :Float64,
    sigma      :Float64,
    steps      :Int64,
    legs       :Int64,
    confirmed  :Bool,
    found      :Bool
  );
}
