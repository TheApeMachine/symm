using Go = import "/go.capnp";
@0xc1b962b023871028;
$Go.package("transport");
$Go.import("nomagique/transport");

struct WireBearerAuth { headers @0 :AnyPointer; token @1 :Text; }

interface BearerAuth {
  write @0 (payload :WireBearerAuth) -> stream;
  done @1 ();
}
