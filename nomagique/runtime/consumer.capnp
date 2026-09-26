using Go = import "/go.capnp";
@0xbaf9a2e8c650d341;
$Go.package("runtime");
$Go.import("github.com/theapemachine/symm/nomagique/runtime");
using import "status.capnp".Status;
using import "status.capnp".Configured;
using import "../ui/binding.capnp".Receiver;
using import "snapshot.capnp".Snapshot;
using import "snapshot.capnp".Checkpoint;

# Results preserve the producing node's actual Cap'n Proto done schema.
struct Result {
 producer @0 :Text;
 node @1 :Text;
 interfaceId @2 :UInt64;
 value @3 :AnyPointer;
 epoch @4 :Int64;
 sequence @5 :Int64;
}

enum Stamp { value @0; epoch @1; sequence @2; }

struct Binding {
 producer @0 :Text;
 node @1 :Text;
 field @2 :Text;
 target @3 :Text;
 stamp @4 :Stamp;
 # A gate requires this output to be present before the stage may evaluate.
 gate @5 :Bool;
}

# A completed call means the existing node graph finished this observation.
struct Completion { outputs @0 :List(Result); bindings @1 :Data; data @2 :Data; }

interface Stage $Go.name("StageNode") {
 step @0 (epoch :Int64, sequence :Int64, data :Data, entry :Text,
          upstream :List(Result), bindings :List(Binding), outputs :List(Text), texts :List(Text), record :Text)
      -> Completion;
 # Fence drains durable state owned by this stage and its children.
 fence @1 () -> ();
}

interface Consumer extends(Stage, Configured) {
 write @0 (target :Stage, entry :Text, name :Text,
           bindings :Text, outputs :List(Text), receiver :Receiver, snapshot :Snapshot, checkpoint :Checkpoint, checkpointReady :Bool, record :Text) -> stream;
 done @1 () -> (epoch :Int64, sequence :Int64, completed :UInt64,
               status :Status, outputs :List(Result), data :Data);
}
