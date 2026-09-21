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
