@0xed13fe1e34dabb68;

using Go = import "/go.capnp";
$Go.package("strategy");
$Go.import("github.com/theapemachine/symm/nomagique/financial/strategy");

interface NormalizeActions {
    write @0 (
        action :Int64,
    ) -> stream;

    done @1 () -> (
        result :Int64,
    );
}
