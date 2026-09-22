@0xa3d7b1e2b9ce8eb4;

using Go = import "/go.capnp";
$Go.package("strategy");
$Go.import("github.com/theapemachine/symm/nomagique/financial/strategy");

interface CountTransactions {
    write @0 (
        action :Int64,
    ) -> stream;

    done @1 () -> (
        count :Int64,
    );
}
