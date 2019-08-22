// "https://cdn.jsdelivr.net/npm/lodash@4.17.15/lodash.min.js"

if (!Array.isArray) {
	Array.isArray = function(arg) {
		return Object.prototype.toString.call(arg) === '[object Array]';
	};
}

/* ----------------------------------------------- */
/* ----------------------------------------------- */

var ws = WSRPC("/kafka/ws", true);
var responseArea = document.getElementById("resp");
var responses = [];


function demoSend() {
	var reqRaw = document.getElementById("request").value;
	var req = JSON.parse(reqRaw);

	var args = {
		callback: onResponse,
		catchCallback: onError,
		finalCallback: onResponse,
	};

	var type;
	if (Array.isArray(req)) {
		var calls = [];

		_.forEach(req, r => {
			if (!!type && r.type !== type) {
				type = 'INVALID';
				return
			}
			type = r.type;

			calls.push({
				type: r.type,
				method: r.method,
				params: r.params,
				header: r.header,
			})
		});

		args.calls = calls;
	} else if (typeof req === 'object') {
		type = req.type;

		args.type = req.type;
		args.header = req.header;
		args.method = req.method;
		args.params = req.params;
	}

	switch (type) {
		case 'STREAM':
			ws.streamrx(args);
			break;
		case 'CALL':
			ws.call(args);
			break;
		default:
			console.log("bad request type(s)");
	}

}

function printResult(res) {
	responses.unshift({
		jobId: res.id,
		batchId: res.batchId,
		// headers: !!res.header ? res.header : undefined,
		result: !res.error ? res.result : res.error,
	});

	responseArea.innerText = JSON.stringify(responses, null, 2);
}

function onResponse(res) {
	if (Array.isArray(res)) {
		_.forEach(res, printResult)
	} else {
		printResult(res)
	}
}

function onError(err) {
	delete(err.result);

	printResult(err);
}
