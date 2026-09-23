@0xdeb1c5afe904dc3f;

using Go = import "/go.capnp";
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

# Queue holds values waiting their turn and hands out one per release.
#
# Every port gathers, so any of them may arrive alone and the queue is asked on
# every evaluation. Each arrival on offer, retry and rewind is a JSON array.
# Offered values join the queue once: a value the queue already knows is not
# queued again. Retried values rejoin at the back. Any arrival on rewind queues
# every known value again, in the order they were first offered. Each arrival
# on release hands out the oldest waiting value; with nothing waiting the
# queue is empty. Within one evaluation rewind applies first, then offers and
# retries, then releases.
interface Queue {
  write @0 (offer :List(Data), retry :List(Data), rewind :List(Data), release :List(Data)) -> stream;
  done @1 () -> Released;
}

struct Released {
  union { empty @0 :Void; out @1 :Data; }
  waiting @2 :UInt64;
  known @3 :UInt64;
}
