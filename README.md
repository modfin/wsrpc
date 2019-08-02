# Web Socket RPC
A simple web socket api framework

## General guidelines for cohabitation (Ok.. rules)
### Server
* Registered handlers are responsible for checking the provided input
* Stream handlers channels go straight to client, mind your output

### Client 
* Callbacks are run before promise handlers in <*connection*>.call()

## Issues
* The error handling could probably be done with middleware, alternatively a logger could be attached
* Attaching request IDs to each context will make debugging and error tracing easier 
* A data race occurs for both wrapped channel types when a stream handler is called with long polling.