@0xbd36ccf72eb932ae;
using Go = import "/go.capnp";
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");
using WireManifoldReading = import "../learning/resonance_manifold.capnp".WireManifoldReading;
using WireRLSOutput = import "../learning/resonance_manifold.capnp".WireRLSOutput;
struct WireResolution {
 prediction @0 :Float64;
 target @1 :Float64;
 error @2 :Float64;
 horizon @3 :Int64;
 step @4 :Int64;
}

# Owns retained predictive parameters and pending causal reference observations.
using Snapshot = import "../runtime/snapshot.capnp".Snapshot;
interface Resonance extends(Snapshot) {
 write @0 (features :List(Float64), reference :Float64, epoch :Int64,
           sequence :Int64, timestamp :Float64, featureIdentities :List(Text)) -> stream;
 done @1 () -> (reading :WireManifoldReading, forecast :List(WireRLSOutput),
  forwardCurve :List(Float64), forwardRetention :List(Float64),
  supportedHorizon :Int64, calibrated :Bool, resolvedSteps :Int64,
  pending :Int64, readout :List(Float64), confidence :Float64,
  lastResolution :WireResolution, alpha :Float64,
  values :List(Float64), present :List(Bool), epoch :Int64, sequence :Int64);
}

# The checkpoint is a native Cap'n Proto message, including unresolved causal rows.
struct ResonanceState {
 epoch @0 :Int64;
 sequence @1 :Int64;
 observations @2 :Int64;
 resolved @3 :Int64;
 reference @4 :Float64;
 returnCount @5 :Int64;
 returnMean @6 :Float64;
 returnM2 @7 :Float64;
 width @8 :UInt32;
 generative @9 :List(ResonanceMatrix);
 recognition @10 :List(ResonanceMatrix);
 temporal @11 :List(ResonanceMatrix);
 latents @12 :List(ResonanceVector);
 previous @13 :List(ResonanceVector);
 heads @14 :List(ResonanceHead);
 pending @15 :List(ResonanceReference);
 memory @16 :ResonanceMemory;
 featureIdentities @17 :List(Text);
}
struct ResonanceVector { values @0 :List(Float64); }
struct ResonanceMatrix { rows @0 :UInt32; columns @1 :UInt32; values @2 :List(Float64); }
struct ResonanceHead {
 beta @0 :List(Float64);
 inverse @1 :ResonanceMatrix;
 nullSpace @2 :ResonanceMatrix;
 rank @3 :Int64;
 observations @4 :Int64;
 residual @5 :Float64;
 support @6 :Int64;
 modelLoss @7 :Float64;
 baselineLoss @8 :Float64;
}
struct ResonanceReference {
 reference @0 :Float64;
 features @1 :List(Float64);
 predictions @2 :List(WireRLSOutput);
 issued @3 :Int64;
 noise @4 :Float64;
}
struct ResonanceMemory {
 previous @0 :List(Float64);
 left @1 :List(Float64);
 right @2 :List(Float64);
 count @3 :Int64;
 leftEnergy @4 :Float64;
 rightEnergy @5 :Float64;
 covariance @6 :Float64;
}
