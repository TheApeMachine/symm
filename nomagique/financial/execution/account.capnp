@0xf49e894ae4664ad6;
using Go = import "/go.capnp";
$Go.package("execution");
$Go.import("github.com/theapemachine/symm/nomagique/financial/execution");
using import "../paper/book.capnp".Market;
using import "../paper/sweep.capnp".Sweep;
using import "../kraken/terms.capnp".Terms;
using import "../../runtime/snapshot.capnp".Snapshot;

# One quote wallet across markets. Decisions reserve funds on the observation
# that caused them; fills require a strictly later reconciled L3 observation.
# Paper probes use the venue's minimum executable quantity/cost, never a fixed
# allocation fraction. Flat and held predictions refer to the same causal cut.
interface Account extends(Snapshot, import "../../runtime/status.capnp".Durable) {
 write @0 (market :Market, epoch :Int64, sequence :Int64,
           flat :List(Data), held :List(Data), terms :Terms, sweep :Sweep,
           initialCash :Text, time :Text, termsReady :Bool, sweepReady :Bool,
           live :Bool, orders :import "../kraken/orders.capnp".Orders,
           ordersReady :Bool, authorized :Bool, durable :Bool,
           checkpoint :import "../../runtime/snapshot.capnp".Checkpoint,
           checkpointKey :Text) -> stream;
 done @1 () -> AccountState;
}

struct AccountState {
 cash @0 :Text;
 pnl @1 :Text;
 equity @2 :Text;
 observations @3 :UInt64;
 decisions @4 :UInt64;
 open @5 :UInt64;
 outcomes @6 :UInt64;
 positives @7 :UInt64;
 meanEdge @8 :Float64;
 edgeDefined @9 :Bool;
 standardError @10 :Float64;
 uncertaintyDefined @11 :Bool;
 phase @12 :Text;
 positions @13 :List(Position);
 closed @14 :RoundTrip;
 decision @15 :Text;
 symbol @16 :Text;
 reason @17 :Text;
 revision @18 :UInt64;
 initialCash @19 :Text;
 edgeM2 @20 :Float64;
 record @21 :import "../../types/record.capnp".Record;
 epoch @22 :Int64;
 sequence @23 :Int64;
 live @24 :Bool;
 durableRevision @25 :UInt64;
 persistenceError @26 :Text;
}
struct Position {
 symbol @0 :Text;
 quantity @1 :Text;
 basis @2 :Text;
 spent @3 :Text;
 proceeds @4 :Text;
 mark @5 :Text;
 pending @6 :Text;
 amount @7 :Text;
 reserved @8 :Text;
 fee @9 :Text;
 minimumQuantity @10 :Text;
 minimumCost @11 :Text;
 increment @12 :Text;
 costPlaces @13 :UInt32;
 epoch @14 :Int64;
 sequence @15 :Int64;
 opened @16 :Text;
 orderId @17 :Text;
 clientId @18 :Text;
 filledQuantity @19 :Text;
 filledCost @20 :Text;
 filledFee @21 :Text;
 intentRevision @22 :UInt64;
}
struct RoundTrip {
 symbol @0 :Text;
 basis @1 :Text;
 proceeds @2 :Text;
 pnl @3 :Text;
 edge @4 :Float64;
 epoch @5 :Int64;
 sequence @6 :Int64;
 opened @7 :Text;
 closed @8 :Text;
}
