package fasthttpadaptor

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/labstack/gommon/log"
	"github.com/valyala/fasthttp"
)

func NewHijackFastHTTPHandler(h http.HandlerFunc) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		var r http.Request
		if err := ConvertRequest(ctx, &r, true); err != nil {
			ctx.Logger().Printf("cannot parse requestURI %q: %v", r.RequestURI, err)
			ctx.Error("Internal Server Error", fasthttp.StatusInternalServerError)
			return
		}

		ctx.HijackSetNoResponse(true)
		ctx.Hijack(func(c net.Conn) {
			w := hijackResponseWriter{
				req:  &r,
				res:  &ctx.Response,
				conn: c,
				bw:   bufio.NewWriter(c),
				br:   bufio.NewReader(c),
			}
			h.ServeHTTP(&w, r.WithContext(ctx))
			fmt.Println("handler return")
		})
	}
}

var _ http.Hijacker = (*hijackResponseWriter)(nil)

type hijackResponseWriter struct {
	req        *http.Request
	statusCode int
	h          http.Header
	res        *fasthttp.Response

	bw *bufio.Writer
	br *bufio.Reader

	conn             net.Conn
	writeContinueMu  sync.Mutex
	canWriteContinue atomic.Bool
	wroteHeader      bool

	statusBuf [3]byte
}

func (w *hijackResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, &bufio.ReadWriter{
		Reader: w.br,
		Writer: w.bw,
	}, nil
}

func (w *hijackResponseWriter) Header() http.Header {
	if w.h == nil {
		w.h = make(http.Header)
	}
	return w.h
}

func (w *hijackResponseWriter) Write(bytes []byte) (int, error) {
	return w.bw.Write(bytes)
}

func (w *hijackResponseWriter) WriteHeader(code int) {
	fmt.Println("write header", code)
	if w.wroteHeader {
		log.Errorf("http: superfluous response.WriteHeader called")
		return
	}

	// Handle informational headers
	if code >= 100 && code <= 199 {
		writeStatusLine(w.bw, w.req.ProtoAtLeast(1, 1), code, w.statusBuf[:])

		// Per RFC 8297 we must not clear the current header map
		w.bw.Write(crlf)
		err := w.bw.Flush()
		if err != nil {
			fmt.Println("bw.Flush", err)
		}

		return
	}

	w.wroteHeader = true
	w.statusCode = code
}

// writeStatusLine writes an HTTP/1.x Status-Line (RFC 7230 Section 3.1.2)
// to bw. is11 is whether the HTTP request is HTTP/1.1. false means HTTP/1.0.
// code is the response status code.
// scratch is an optional scratch buffer. If it has at least capacity 3, it's used.
func writeStatusLine(bw *bufio.Writer, is11 bool, code int, scratch []byte) {
	if is11 {
		bw.WriteString("HTTP/1.1 ")
	} else {
		bw.WriteString("HTTP/1.0 ")
	}
	if text := http.StatusText(code); text != "" {
		bw.Write(strconv.AppendInt(scratch[:0], int64(code), 10))
		bw.WriteByte(' ')
		bw.WriteString(text)
		bw.Write(crlf)
	} else {
		// don't worry about performance
		fmt.Fprintf(bw, "%03d status code %d\r\n", code, code)
	}
}

var (
	crlf = []byte("\r\n")
)
