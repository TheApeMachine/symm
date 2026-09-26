@0xc3bac7c120d00a4d;
using Go = import "/go.capnp";
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");
using import "../financial/paper/book.capnp".Market;

# Explicit retained owner of the existing Sensorium coupled physical domain.
# One reconciled L3 order queue updates the corresponding symbol's particles.
interface Manifold {
 write @0 (symbol :Text, market :Market, excitation :List(Float64), present :List(Bool),
           epoch :Int64, sequence :Int64, gridX :UInt32, gridY :UInt32, gridZ :UInt32) -> stream;
 done @1 () -> (frame :ManifoldFrame, values :List(Float64), present :List(Bool),
                 epoch :Int64, sequence :Int64);
}
struct ManifoldFrame {
 epoch @0 :Int64;
 sequence @1 :Int64;
 version @2 :UInt64;
 gridX @3 :UInt32;
 gridY @4 :UInt32;
 gridZ @5 :UInt32;
 spacing @6 :Float64;
 population @7 :UInt32;
 positions @8 :List(Float32);
 velocities @9 :List(Float32);
 masses @10 :List(Float32);
 energies @11 :List(Float32);
 phases @12 :List(Float32);
 frequencies @13 :List(Float32);
 amplitudes @14 :List(Float32);
 heat @15 :List(Float32);
 contentIds @16 :List(Int64);
 densityMomentum @17 :List(Float32);
 fieldEnergy @18 :List(Float32);
 waveReal @19 :List(Float32);
 waveImaginary @20 :List(Float32);
 divergence @21 :Float64;
 guidanceSpeed @22 :Float64;
 coherence @23 :Float64;
 pressureGradient @24 :Float64;
 viscosity @25 :Float64;
 synchronization @26 :Float64;
 physicalTime @27 :Float64;
 acceptedStep @28 :Float64;
 substeps @29 :UInt32;
 densityScale @30 :Float32;
 momentumScale @31 :Float32;
 energyScale @32 :Float32;
 waveScale @33 :Float32;
}
