using Go = import "/go.capnp";
@0xdf0315c46f0719ed;
$Go.package("http");
$Go.import("github.com/theapemachine/symm/nomagique/network/http");

using import "../../runtime/status.capnp".Status;

using import "../../store/tables/query.capnp".Query;

using import "../../ui/binding.capnp".Receiver;

interface HTTPServer extends(Receiver) {
  write @0 (data :Data, path :Text, status :Int32, query :Query, routes :Text, address :Text) -> stream;
  done @1 () -> (status :Status, out :Data);
}
