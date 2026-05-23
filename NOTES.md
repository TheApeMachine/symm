This is what we are trying to do. We are not trying to "predict" the market, that would be foolish. Our main focus is on trying to find enough signal to ride certain structural events on the market.

1. Pump & Dumps - Here we would often see a rapid spike up, and a rapid drop down, and by rapid I mean usually in seconds. These fluctuations are often multiple hundreds of percents, and repeat a couple of times once they start.

2. Meme/Alt coins - As you can see from the screenshots, here we are talking about either established coins that somehow starat rising by big percentages in 24 hours, or often also newly listed coins that have a similar significant bump upwards initially.

3. Coins with enough volatility that we can nibble constant small profits from, while the system waits for the next big event to take place.

We already wrote some versions in Python, which have seen some success (paper trading though).
I will not tell you the strategies yet, because I woudl like to see if we have any new ideas. I might introduce the current strategies one by one later.

One thing I will tell you. This system needs to be responsive, definitely needs multiple signals it can evaluate, and most importantly, not rely too much on static values (magic numbers, guesses, etc.)
The market conditions fluctuate and the system needs to calibrate itself dynamically.

And finally one more thing. By far the best result are achieved by using moving stop-losses when entering a trade. So initial enter, set the stop-loss at some logical position, but if the symbol pair keeps rising, keep moving the stop-loss up as well, as this will lock in certain minimum returns.
Of course in paper trading mode you will have to accurately simulate the stop-loss system.

Also, don't forget to accurately account for slippage, and fees.

You can find out what you have available in data here:

https://docs.kraken.com/api/docs/websocket-v2/add_order 

and here:

https://docs.kraken.com/api/docs/rest-api/add-order 

Using the WebSocket API is probably preferred for speed/reaction time, but it does not have everything the HTTP API has