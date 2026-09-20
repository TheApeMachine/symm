using Go = import "/go.capnp";
@0xc7e14a3b8d9a2c1f;
$Go.package("tables");
$Go.import("nomagique/store/tables");

interface IcebergTable {
  write @0 (config :Text, payload :Data) -> stream;
  done @1 () -> (out :Data);
}

interface IcebergScan {
  write @0 (config :Text, query :Data) -> stream;
  done @1 () -> (out :Data);
}
