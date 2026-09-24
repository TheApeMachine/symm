@0xbac867932854ed01;
using Go = import "/go.capnp";
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

using import "../runtime/status.capnp".Queued;

# Collect a snapshot, deduplicate capture identities, then replay each session
# by numeric sequence. Sessions are independent tapes, not a global clock.
# rows gathers: every archive row handed over together is collected at once,
# so a snapshot read a batch at a time is collected a batch at a time.
#
# Once sealed, each evaluation replays one tick of one session's capture
# clock: from the next frame in sequence order up to the first frame received
# once the clock has passed the second that frame was received in, slot n of
# every list describing one frame. Sockets are read side by side, so a
# lagging one's frames may carry earlier receive times and still belong to
# the run. How many frames an evaluation carries is how many the recording
# holds, and a consumer stepping them in order sees exactly what it would one
# at a time. documents holds each frame as a document: its capture identity under
# capture (session, endpoint, receivedAt), its sequence under cursor, and the
# frame itself under the field envelope names. pending counts the frames
# still to replay.
interface Tape extends(Queued) {
 write @0 (rows :List(Data), exhausted :Bool, envelope :Text) -> stream;
 done @1 () -> TapeResult;
}
struct TapeResult {
 finished @0 :Bool;
 pending @1 :UInt64;
 union {
  idle @2 :Void;
  frames :group {
   session @3 :Text;
   payload @4 :List(Data);
   sequence @5 :List(Int64);
   receivedAt @6 :List(Text);
   endpoint @7 :List(Text);
   documents @8 :List(Data);
  }
  exhausted @9 :Void;
 }
}
