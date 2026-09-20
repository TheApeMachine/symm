using Go = import "/go.capnp";
@0xffb921aa3b427c35;
$Go.package("execution");
$Go.import("nomagique/execution");

struct WireGate {
  action @0 :Text;
}

interface Gate {
  write @0 (gate :WireGate) -> stream;
  done @1 ();
}
