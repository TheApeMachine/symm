using Go = import "/go.capnp";
@0xb09354f25f9df495;
$Go.package("execution");
$Go.import("nomagique/execution");

interface Submit {
  write @0 (
    action :Text,
    symbol :Text
  ) -> stream;
  done @1 () -> (
    out :Data,
    symbol :Text,
    action :Text,
    status :Text,
    timestamp :Int64
  );
}
