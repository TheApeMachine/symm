@0xc7a241a6668d5a49;

using Go = import "/go.capnp";
$Go.package("strategy");
$Go.import("github.com/theapemachine/symm/nomagique/financial/strategy");

interface Or {
    write @0 (
        action1 :Int64,
        action2 :Int64,
    ) -> stream;

    done @1 () -> (
        action :Int64,
    );
}
