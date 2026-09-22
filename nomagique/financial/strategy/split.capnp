@0xd91120371eb71358;

using Go = import "/go.capnp";
$Go.package("strategy");
$Go.import("github.com/theapemachine/symm/nomagique/financial/strategy");

interface Split {
    write @0 (
        buyAction :Int64,
        sellAction :Int64,
    ) -> stream;

    done @1 () -> (
        action :Int64,
    );
}
