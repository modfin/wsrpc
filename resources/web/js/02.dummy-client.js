// "https://cdn.jsdelivr.net/npm/lodash@4.17.15/lodash.min.js"

if (!Array.isArray) {
	Array.isArray = function(arg) {
		return Object.prototype.toString.call(arg) === '[object Array]';
	};
}

/* ----------------------------------------------- */
/* ----------------------------------------------- */

var ws = WSRPC("/kafka/ws", false);
var responseArea = document.getElementById("resp");
var responses = {};


function demoSend() {
	var reqRaw = document.getElementById("request").value;
	var req = JSON.parse(reqRaw);

	var args = {
		callback: onResponse,
		catchCallback: onError,
		finalCallback: onResponse,
	};

	if (Array.isArray(req)) {
		var calls = [];

		_.forEach(req, r => {
			args.type = r.type;

			calls.push({
				method: r.method,
				params: r.params,
				header: r.header,
			})
		});

		args.calls = calls;
	} else if (typeof req === 'object') {
		args.type = req.type;
		args.header = req.header;
		args.method = req.method;
		args.params = req.params;
	}


	switch (args.type) {
		case 'STREAM':
			ws.streamrx(args);
			break;
		case 'CALL':
			ws.call(args);
			break;
		default:
			console.log("bad request type");
	}

}

function printResult(res) {
	responses[res.id] = {
		headers: res.header,
		result: res.result ? res.result : res.error,
	};

	responseArea.innerText = JSON.stringify(responses, null, 2);
}

function onResponse(res) {
	if (Array.isArray(res)) {
		_.forEach(res, r => {
			delete(res.error);

			printResult(res)
		})
	} else {
		delete(res.error);

		printResult(res)
	}
}

function onError(err) {
	delete(err.result);

	printResult(err);
}
