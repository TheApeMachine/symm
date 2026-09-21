@0xefa7e2459e748f2a;

using Go = import "/go.capnp";
$Go.package("algo");
$Go.import("github.com/theapemachine/symm/nomagique/algo");

using import "../runtime/status.capnp".Status;

# Interval is one log return over the half-open span it was measured across.
struct Interval {
  from  @0 :Float64;
  to    @1 :Float64;
  value @2 :Float64;
}

# Accumulation is the estimator's retained history for one pair of paths.
# It travels through the graph rather than living inside the estimator, so the
# retention is composed by whoever stores it and is visible where it is kept.
struct Accumulation {
  left        @0 :List(Interval);
  right       @1 :List(Interval);
  lastLeft    @2 :Interval;
  lastRight   @3 :Interval;
  leftSeen    @4 :Bool;
  rightSeen   @5 :Bool;
  covariance  @6 :Float64;
  support     @7 :Float64;
  leftEnergy  @8 :Float64;
  rightEnergy @9 :Float64;
}

# HayashiYoshida is the asynchronous covariance of two return paths. It reports
# the estimate and the terms it was formed from, so a caller can audit the
# normalization rather than take the ratio on faith.
#
# The estimator keeps nothing between evaluations: the accumulation it reads is
# handed to it and the updated one is handed back, which is what lets a graph
# hold one history per pair of symbols in explicit storage.
interface HayashiYoshida {
  write @0 (
    state        :Data,
    boundsStart1 :Float64,
    boundsEnd1   :Float64,
    returns1     :Float64,
    boundsStart2 :Float64,
    boundsEnd2   :Float64,
    returns2     :Float64
  ) -> stream;
  done @1 () -> (
    state       :Data,
    correlation :Float64,
    covariance  :Float64,
    support     :Float64,
    leftEnergy  :Float64,
    rightEnergy :Float64,
    status      :Status
  );
}
