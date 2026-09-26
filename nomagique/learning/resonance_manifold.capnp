@0xc0fa48ae932ef25d;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

struct WireRLSOutput {
  value @0 :Float64;
  scale @1 :Float64;
  degreesOfFreedom @2 :Float64;
  ready @3 :Bool;
  innovation @4 :Float64;
  reset @5 :Bool;
}

struct WireResonanceLayer {
  state @0 :List(Float64);
  prediction @1 :List(Float64);
  errorNorm @2 :Float64;
  temporal @3 :Bool;
}

struct WireManifoldReading {
  reconstruction @0 :List(Float64);
  energy @1 :Float64;
  predictionEnergy @2 :Float64;
  reconstructionError @3 :Float64;
  energyDensity @4 :Float64;
  surprise @5 :Float64;
  temporalError @6 :Float64;
  hasTemporalError @7 :Bool;

  readoutDimension @8 :Int64;
  readout @9 :List(Float64);
  latent @10 :List(Float64);
  taskPrediction @11 :List(Float64);
  layers @12 :List(WireResonanceLayer);

  skill @13 :List(Float64);
  skillReady @14 :List(Bool);
  skillAverage @15 :Float64;
  skillReadyAvg @16 :Bool;
  precisionReady @17 :List(Bool);
  precisionAverage @18 :Float64;
  precisionReadyAvg @19 :Bool;
  scaleAverage @20 :Float64;
  scaleReadyAvg @21 :Bool;

  forecast @22 :List(WireRLSOutput);
  retention @23 :List(Float64);
}

