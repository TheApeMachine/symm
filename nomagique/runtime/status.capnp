using Go = import "/go.capnp";
@0x91d758a99e934525;
$Go.package("runtime");
$Go.import("github.com/theapemachine/symm/nomagique/runtime");

enum Status {
  init @0;
  ok @1;
  error @2;
  fatal @3;
  ready @4;
  busy @5;
  waiting @6;
  done @7;
}

struct StatusPayload {
  status @0 :Status;
}

# Source owns external observations. Nonempty Data results indicate work
# received from outside the graph, rather than recirculated graph values.
interface Source {}

# Durable owners flush their pending writes explicitly before capabilities release.
interface Durable {
  flush @0 () -> ();
}

# Queued owners report pending :UInt64 in done. A positive count means
# admitted work remains runnable without a new external observation.
interface Queued {}

# Standing owners report on every evaluation, whether or not anything reached
# them, so a consumer may join on what they say when they were told nothing.
interface Standing {}
