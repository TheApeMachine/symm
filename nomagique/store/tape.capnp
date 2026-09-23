@0xbac867932854ed01;
using Go = import "/go.capnp";
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

using import "../runtime/status.capnp".Queued;

# Collect a snapshot, deduplicate capture identities, then replay each session
# by numeric sequence. Sessions are independent tapes, not a global clock.
# Once sealed, pending counts the frames still to replay.
interface Tape extends(Queued) {
 write @0 (row :Data, exhausted :Bool) -> stream;
 done @1 () -> TapeResult;
}
struct TapeResult {
 finished @7 :Bool;
 pending @9 :UInt64;
 union {
  idle @0 :Void;
  frame :group {
   payload @1 :Data;
   session @2 :Text;
   sequence @3 :Int64;
   receivedAt @4 :Text;
   endpoint @5 :Text;
   row @8 :Data;
  }
  exhausted @6 :Void;
 }
}
