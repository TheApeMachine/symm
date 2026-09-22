@0xe4ef36d99ff367f5;

using Go = import "/go.capnp";
$Go.package("trend");
$Go.import("github.com/theapemachine/symm/nomagique/financial/trend");

interface Mls {
    write @0 (
        x :Float64,
        y :Float64,
    ) -> stream;

    done @1 () -> (
        m :Float64,
        b :Float64,
    );
}
