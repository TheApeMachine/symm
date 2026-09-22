using Go = import "/go.capnp";
@0xb6ad448353121c47;
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

interface Process {
  write @0 (time :Float64, mark :Float64) -> stream;
  done @1 () -> (
    eventCount      :Float64,
    buyCount        :Float64,
    sellCount       :Float64,
    buyFraction     :Float64,
    sellFraction    :Float64,
    arrivalRate     :Float64,
    buyRate         :Float64,
    sellRate        :Float64,
    lambda          :Float64,
    lambdaBuy       :Float64,
    lambdaSell      :Float64,
    spectralRadius  :Float64,
    muBuy           :Float64,
    muSell          :Float64,
    mu              :Float64,
    excessBuy       :Float64,
    excessSell      :Float64,
    excitationBuy   :Float64,
    excitationSell  :Float64,
    alphaBuyBuy     :Float64,
    alphaBuySell    :Float64,
    alphaSellBuy    :Float64,
    alphaSellSell   :Float64,
    beta            :Float64,
    timescale       :Float64,
    offspringBuyBuy   :Float64,
    offspringBuySell  :Float64,
    offspringSellBuy  :Float64,
    offspringSellSell :Float64,
    descendantsBuy  :Float64,
    descendantsSell :Float64,
    compensatorBuy  :Float64,
    compensatorSell :Float64,
    innovationBuy   :Float64,
    innovationSell  :Float64,
    snr             :Float64,
    logLikelihoodHawkes   :Float64,
    logLikelihoodPoisson  :Float64,
    logLikelihoodSelfOnly :Float64
  );
}
