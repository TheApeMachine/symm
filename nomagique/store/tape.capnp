@0xbac867932854ed01;
using Go = import "/go.capnp";
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

# Collect a snapshot, deduplicate capture identities, then replay each session
# by numeric sequence. Sessions are independent tapes, not a global clock.
interface Tape {
 write @0 (row :Data, exhausted :Bool) -> stream;
 done @1 () -> TapeResult;
}
struct TapeResult {
 finished @7 :Bool;
 union {
  idle @0 :Void;
  frame :group {
   payload @1 :Data;
   session @2 :Text;
   sequence @3 :Int64;
   receivedAt @4 :Text;
   endpoint @5 :Text;
  }
  exhausted @6 :Void;
 }
}
