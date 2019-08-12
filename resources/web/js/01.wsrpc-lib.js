// TODO polyfill Promise API

function WSRPC(url, disableWebsocket) {
	var ws;

	if (url[0] === '/') {
		url = window.location.host + url
	}
	var wsUrl = (window.location.protocol === 'https:' ? 'wss://' : 'ws://') + url;
	var httpUrl = window.location.protocol + '://' + url;

	var websocketEnabled = true;
	var connected = false;

	var taskCounter = 0;
	var batchCounter = 0;
	var stack = [];
	var resolvers = {};
	var batches = {};

	var timeout = 10;
	var errCount = 0;

	function newPayload(type, method, params) {
		return {
			jsonrpc: "2.0",
			id: ++taskCounter,
			type: type,
			method: method,
			params: params
		}
	}

	function send(data) {
		if (!!ws && websocketEnabled) {
			ws.send(data);
			return
		}

		function onError(err) {
			console.log("internal err: ", err);
			return undefined
		}

		fetch(httpUrl, {
			method: 'POST',
			body: data,
		}).then(
			function (res) {
				res.json().then(onmessage, onError)
			},
			function (err) {
				err.json().then(onmessage, onError)
			}
		)
	}

	function onmessage(data) {
		if (Array.isArray(data)) {
			data.forEach(parseMessage);
			return
		}
		parseMessage(data);
	}


	function connect() {
		if (disableWebsocket) {
			return
		}

		ws = new WebSocket(wsUrl);
		taskCounter = 0;

		ws.onopen = function () {
			timeout = 10;
			connected = true;
			while (stack.length > 0) {
				send(stack.shift())
			}
		};

		ws.onclose = function () {
			connected = false;
			reconnect()
		};

		// used for debugging
		var states = {
			3: "closed",
			2: "closing",
			1: "open",
			0: "connecting"
		};
		ws.onerror = function () {
			errCount++;

			switch (ws.readyState) {
				case WebSocket.OPEN:
					console.log("confucious says WTF?");
					break;
				case WebSocket.CONNECTING:
				case WebSocket.CLOSING:
				case WebSocket.CLOSED:
				default:
					connected = false;
			}
		};

		ws.onmessage = function (message) {
			var data = JSON.parse(message.data);
			onmessage(data)
		};
	}

	function reconnect() {
		setTimeout(function () {
			timeout = timeout > 10000 ? 10000 : timeout * 10;
			connect()
		}, timeout)
	}

	function parseMessage(obj) {
		var resolver = resolvers[obj.id];
		if (!resolver) {
			console.log('found stray msg: ', obj);
			return
		}

		if (resolver.type === 'call') {
			delete resolvers[obj.id];
			if (!!obj.error) {
				if (!!resolver.catchCallback) {
					resolver.catchCallback(obj)
				}
				resolver.reject(obj);
				return;
			}


			if (resolver.callback) {
				resolver.callback(obj)
			}
			resolver.resolve(obj);
			return;
		}

		if (resolver.type === 'stream') {
			if (!!obj.error) {
				if (obj.error.code === 205) {
					delete resolvers[obj.id];
					if (!!resolver.finalCallback) {
						resolver.finalCallback(obj)
					}
					return
				}

				if (resolver.catchCallback) {
					resolver.catchCallback(obj)
				}
				return;
			}

			var cancel = false;
			if (resolver.callback) {
				resolver.callback(obj, function(){cancel = true})
			}

			if (cancel) {
				return
			}

			if (!websocketEnabled) {
				console.log('batches', JSON.stringify(batches, null, 2));
				console.log('resolvers ', JSON.stringify(resolver, null, 2));
				var watchers;
				if (resolver.batchId !== undefined) {
					watchers = batches[resolver.batchId];
				} else {
					watchers = [];
					watchers.push(obj.id)
				}

				console.log('found: ', resolver.batchId, ' have ', JSON.stringify(watchers));
				watchers.forEach(function(id) {
					var payload = resolvers[id].payload;

					if (id === obj.id) {
						payload.header = payload.header || {};
						payload.header.State = obj.header.State;
					}

					var data = JSON.stringify(payload);
					if (connected) {
						send(data);
					} else {
						stack.push(data)
					}
				})
			}
		}
	}

	// If we receive too many errors over a short period of time we consider the web socket unstable and switch to long polling
	function checkConnectivity() {
		if (errCount > 10) {
			websocketEnabled = false;
			clearStack()
		}

		errCount = 0;
		setTimeout(checkConnectivity, 5000);
	}
	if (!websocketEnabled) {
		checkConnectivity();
	}


	connect();


	function clearStack() {
		if (!connected && !websocketEnabled) {
			while (stack.length > 0) {
				var data = stack.shift()
				send(data);
			}
		}

		setTimeout(function() {
			if (!websocketEnabled) {
				clearStack()
			}
		}, 100);
	}
	// clearStack();


	return {
		// args is assumed to be an object containing
		// 1. Either/Both []{method, params} or method, params. Where params is optional for all methods and calls
		// 2. a callback for successful calls
		// 3. a callback for error handling
		call: function (args) {
			if (!!args.method) {
				args.calls = args.calls || [];

				args.calls.push({
					method: args.method,
					params: args.params,
				})
			}

			var payloads = [];
			var promises = [];
			args.calls.forEach(function (call) {
				var p = new Promise(function (resolve, reject) {
					var payload = newPayload('CALL', call.method, call.params);
					payloads.push(payload);

					resolvers[payload.id] = {
						type: 'call',
						args: args,
						resolve: resolve,
						reject: reject,
						callback: args.callback,
						catchCallback: args.catchCallback
					}
				});

				promises.push(p)
			});

			var data;
			if (payloads.length > 1) {
				data = JSON.stringify(payloads);
			}
			if (payloads.length === 1) {
				data = JSON.stringify(payloads[0])
			}

			if (connected) {
				send(data)
			} else {
				stack.push(data)
			}

			return promises.length > 1 ? promises : promises[0];
		},
		streamrx: function (args) {
			console.log('!!args.calls', !!args.calls);
			if (!!args.calls && args.calls.length > 1) {

				args.batchId = batchCounter++;
				batches[args.batchId] = [];
			}
			if (!!args.method) {
				args.calls = args.calls || [];

				args.calls.push({
					method: args.method,
					params: args.params,
				})
			}

			var payloads = [];
			args.calls.forEach(function (call) {
				var payload = newPayload('STREAM', call.method, call.params);
				payloads.push(payload);

				if (args.batchId !== undefined) {
					batches[args.batchId].push(payload.id);
				}

				resolvers[payload.id] = {
					type: 'stream',
					payload: payload,
					batchId: args.batchId,
					callback: args.callback,
					catchCallback: args.catchCallback,
					finalCallback: args.finalCallback,
				};
			});
			console.log(JSON.stringify(resolvers, null, 2))

			var data;
			if (payloads.length > 1) {
				data = JSON.stringify(payloads);
			}
			if (payloads.length === 1) {
				data = JSON.stringify(payloads[0])
			}

			if (connected) {
				send(data);
			} else {
				stack.push(data)
			}

		},
	}
}

