using Go = import "/go.capnp";
@0xd28f099c0a6f44d9;
$Go.package("data");
$Go.import("nomagique/data");

struct WireQualityFacts {
  support @0 :Float64;
  divergence @1 :Float64;
  noiseVariance @2 :Float64;
  mahalanobisSNR @3 :Float64;
  maturity @4 :Float64;
  hasSupport @5 :Bool;
  hasDivergence @6 :Bool;
  hasNoise @7 :Bool;
  hasMahalanobis @8 :Bool;
  hasMaturity @9 :Bool;
}

struct WireQualityReading {
  snr @0 :Float64;
  snrDefined @1 :Bool;
  estimated @2 :Bool;
  maturity @3 :Float64;
}

interface Quality {
  evaluate @0 (facts :WireQualityFacts) -> (reading :WireQualityReading);
}
