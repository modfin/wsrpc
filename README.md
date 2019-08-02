# Web Socket RPC
A simple web socket api framework, made for subscribing to event history in kafka

## General guidelines for cohabitation (Ok.. rules)
### Server
* Registered handlers are responsible for checking the provided input
* Stream handlers channels go straight to client, mind your output

### Client 
* Callbacks are run before promise handlers in <*connection*>.call()