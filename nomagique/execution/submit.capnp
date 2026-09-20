using Go = import "/go.capnp";
@0xb09354f25f9df495;
$Go.package("execution");
$Go.import("nomagique/execution");

struct WireSubmit {
  action @0 :Text;
  symbol @1 :Text;
}

interface Submit {
  write @0 (submit :WireSubmit) -> stream;
  done @1 ();
}
