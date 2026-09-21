using Go = import "/go.capnp";
@0xc1b962b023871028;
$Go.package("transport");
$Go.import("nomagique/transport");

interface BearerAuth {
  write @0 (data :Data) -> stream;
  done @1 () -> (out :Data);
}
