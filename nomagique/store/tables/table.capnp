using Go = import "/go.capnp";
@0xc7e14a3b8d9a2c1f;
$Go.package("tables");
$Go.import("nomagique/store/tables");

struct WireIcebergTable {
  config @0 :Text;
  payload @1 :AnyPointer;
}

struct WireIcebergScan {
  config @0 :Text;
  query @1 :AnyPointer;
}

interface IcebergTable {
  execute @0 (table :WireIcebergTable) -> (result :AnyPointer);
}

interface IcebergScan {
  execute @0 (scan :WireIcebergScan) -> (results :AnyPointer);
}
