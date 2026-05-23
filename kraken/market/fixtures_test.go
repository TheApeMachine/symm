package market

var sampleTradeFrame = []byte(`{
  "channel":"trade",
  "type":"update",
  "data":[
    {"symbol":"BTC/EUR","side":"buy","qty":0.25,"price":50000.1,"timestamp":"2026-05-23T02:00:00.123456789Z"},
    {"symbol":"BTC/EUR","side":"sell","qty":0.10,"price":50000.2,"timestamp":"2026-05-23T02:00:00.223456789Z"}
  ]
}`)

var sampleBookFrame = []byte(`{
  "channel":"book",
  "type":"update",
  "data":[
    {
      "symbol":"BTC/EUR",
      "bids":[{"price":49999.5,"qty":1.2}],
      "asks":[{"price":50000.5,"qty":0.8}]
    }
  ]
}`)

var sampleInstrumentFrame = []byte(`{
  "channel":"instrument",
  "type":"snapshot",
  "data":{
    "assets":[],
    "pairs":[
      {
        "symbol":"BTC/EUR",
        "base":"BTC",
        "quote":"EUR",
        "status":"online",
        "qty_precision":8,
        "qty_increment":1e-8,
        "price_precision":1,
        "cost_precision":5,
        "marginable":true,
        "has_index":true,
        "cost_min":0.45,
        "price_increment":0.1,
        "qty_min":0.0001
      },
      {
        "symbol":"BTC/USD",
        "base":"BTC",
        "quote":"USD",
        "status":"online",
        "qty_precision":8,
        "qty_increment":1e-8,
        "price_precision":1,
        "cost_precision":5,
        "marginable":true,
        "has_index":true,
        "cost_min":0.5,
        "price_increment":0.1,
        "qty_min":0.0001
      }
    ]
  }
}`)
