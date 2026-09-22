using Go = import "/go.capnp";
@0x8f84eb0b54c7b86e;
$Go.package("http");
$Go.import("github.com/theapemachine/symm/nomagique/network/http");

struct Header { name @0 :Text; value @1 :Text; }

# One HTTP exchange. Method, address, headers and body are graph inputs.
interface HTTPClient {
  write @0 (url :Text, method :Text, headers :List(Header), body :Data) -> stream;
  done @1 () -> (out :Data, status :UInt16);
}
