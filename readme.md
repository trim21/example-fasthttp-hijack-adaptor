An example to adaptor `(*fasthttp.RequestCtx).Hijack` to `net/http.Hijacker` for websocket requests.

Not production ready, use on your own risk.


try `go run main.go` and visit <http://127.0.0.1:9999/>,
you should see websocket is working as expected.
