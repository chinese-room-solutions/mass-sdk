// Package gatewayhttp tunnels a runtime gateway's
// [gatewaypb.RuntimeGateway_HandleRequestServer] stream into a normal
// [http.Handler] and streams the response back. Trailers are preserved
// so a gRPC server can ride the same tunnel (gRPC encodes terminal
// status as HTTP/2 trailers). Pass nil for grpcSrv when the gateway has
// no typed gRPC services.
package gatewayhttp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/KernelPryanic/ctxerr"
	gatewaypb "github.com/chinese-room-solutions/mass-proto/gen/go/gateway"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Serve consumes one inbound HandleRequest stream, reconstructs the
// HTTP request, dispatches it to handler (or grpcSrv when the
// content-type is application/grpc*), and streams the response back.
// grpcSrv may be nil — gRPC requests then fall through to handler.
func Serve(stream gatewaypb.RuntimeGateway_HandleRequestServer, handler http.Handler, grpcSrv http.Handler) error {
	if handler == nil {
		return status.Error(codes.FailedPrecondition, "gatewayhttp.Serve: handler is nil")
	}

	first, err := stream.Recv()
	if err != nil {
		return ctxerr.With(fmt.Errorf("receiving first request chunk: %w", err), nil)
	}
	if first.GetMethod() == "" || first.GetPath() == "" {
		return status.Error(codes.InvalidArgument, "first chunk must carry method + path")
	}

	body, err := assembleRequestBody(stream, first)
	if err != nil {
		return err
	}

	// MASS strips its `/mass.<runtime_name>` prefix and forwards the rest
	// verbatim, so typed gRPC paths arrive without a leading slash (e.g.
	// ".v1/Foo"). Force one so http.NewRequestWithContext parses it as
	// absolute and the mux can dispatch.
	reqPath := first.Path
	if reqPath == "" || reqPath[0] != '/' {
		reqPath = "/" + reqPath
	}
	httpReq, err := http.NewRequestWithContext(stream.Context(), first.Method, reqPath, body)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "constructing http request: %v", err)
	}
	for k, v := range first.GetHeaders() {
		httpReq.Header.Set(k, v)
	}

	rw := newStreamResponseWriter(stream)
	if grpcSrv != nil && isGRPCRequest(httpReq) {
		// grpc.Server.ServeHTTP requires HTTP/2, and the reconstructed
		// request defaults to HTTP/1.1. Forging the proto fields
		// satisfies its check without a real h2 connection: the body is
		// already buffered and rw implements http.Flusher, which is all
		// the transport actually needs.
		httpReq.ProtoMajor = 2
		httpReq.ProtoMinor = 0
		httpReq.Proto = "HTTP/2.0"
		grpcSrv.ServeHTTP(rw, httpReq)
	} else {
		handler.ServeHTTP(rw, httpReq)
	}
	return rw.Finish()
}

func isGRPCRequest(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc")
}

// assembleRequestBody buffers the streamed request body fully. Chat
// / embed / tokenize requests are small in practice; streaming
// uploads would need a goroutine-fed pipe, which we'll add when a
// real use case shows up.
func assembleRequestBody(stream gatewaypb.RuntimeGateway_HandleRequestServer, first *gatewaypb.HTTPRequestChunk) (io.ReadCloser, error) {
	var buf bytes.Buffer
	if len(first.GetBody()) > 0 {
		buf.Write(first.GetBody())
	}
	if first.GetEndOfStream() {
		return io.NopCloser(&buf), nil
	}
	for {
		chunk, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.NopCloser(&buf), nil
			}
			return nil, ctxerr.With(fmt.Errorf("receiving request body chunk: %w", err), nil)
		}
		if len(chunk.GetBody()) > 0 {
			buf.Write(chunk.GetBody())
		}
		if chunk.GetEndOfStream() {
			return io.NopCloser(&buf), nil
		}
	}
}

// streamResponseWriter frames every Write into an HTTPResponseChunk
// and implements http.Flusher so SSE / streaming responses travel
// through immediately.
type streamResponseWriter struct {
	stream      gatewaypb.RuntimeGateway_HandleRequestServer
	header      http.Header
	wroteHeader bool
	finished    bool
	finishErr   error
}

func newStreamResponseWriter(stream gatewaypb.RuntimeGateway_HandleRequestServer) *streamResponseWriter {
	return &streamResponseWriter{stream: stream, header: http.Header{}}
}

func (w *streamResponseWriter) Header() http.Header { return w.header }

func (w *streamResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if err := w.stream.Send(&gatewaypb.HTTPResponseChunk{
		Status:  int32(code),
		Headers: flattenResponseHeaders(w.header),
	}); err != nil {
		w.finishErr = err
	}
}

func (w *streamResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.finishErr != nil {
		return 0, w.finishErr
	}
	if len(p) == 0 {
		return 0, nil
	}
	if err := w.stream.Send(&gatewaypb.HTTPResponseChunk{Body: p}); err != nil {
		w.finishErr = err
		return 0, err
	}
	return len(p), nil
}

func (w *streamResponseWriter) Flush() {}

// Finish emits the terminal end-of-stream frame, attaching any
// trailers the handler wrote. gRPC needs this for its grpc-status /
// grpc-message trailers; without them the client sees a hung RPC.
func (w *streamResponseWriter) Finish() error {
	if w.finished {
		return w.finishErr
	}
	w.finished = true
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.finishErr != nil {
		return w.finishErr
	}
	if err := w.stream.Send(&gatewaypb.HTTPResponseChunk{
		EndOfStream: true,
		Trailers:    extractTrailers(w.header),
	}); err != nil {
		return ctxerr.With(fmt.Errorf("sending EOS: %w", err), nil)
	}
	return nil
}

var (
	_ http.ResponseWriter = (*streamResponseWriter)(nil)
	_ http.Flusher        = (*streamResponseWriter)(nil)
)

// flattenResponseHeaders flattens response headers into the initial
// HTTPResponseChunk. Trailer entries (announced via "Trailer:" or
// written under http.TrailerPrefix) are excluded — they ride the
// EndOfStream frame instead.
func flattenResponseHeaders(in http.Header) map[string]string {
	announced := make(map[string]struct{})
	// Same multi-value gotcha as extractTrailers: gRPC adds two
	// separate Trailer headers (one per name), so h.Get returns only
	// the first. Walk h.Values to see both.
	for _, v := range in.Values("Trailer") {
		for k := range strings.SplitSeq(v, ",") {
			k = http.CanonicalHeaderKey(strings.TrimSpace(k))
			if k != "" {
				announced[k] = struct{}{}
			}
		}
	}
	out := make(map[string]string, len(in))
	for k, vs := range in {
		if strings.HasPrefix(k, http.TrailerPrefix) {
			continue
		}
		if _, ok := announced[k]; ok {
			continue
		}
		if k == "Trailer" {
			continue
		}
		out[k] = strings.Join(vs, ",")
	}
	return out
}

// extractTrailers harvests trailer entries per net/http's two
// conventions: pre-declared via the "Trailer:" header (final values
// live under those keys after the handler returns), and late-bound via
// the http.TrailerPrefix magic prefix (stripped here). Returns nil for
// plain HTTP responses with no trailers.
//
// Multi-value "Trailer:" parse trap: net/http handlers may add multiple
// separate Trailer entries (gRPC does this — two
// h.Add("Trailer", "Grpc-Status") and h.Add("Trailer", "Grpc-Message")
// calls), OR set one entry with a comma-separated list. We must walk
// both shapes: h.Values("Trailer") returns every Add'd value, and each
// value may itself be a comma list.
func extractTrailers(h http.Header) map[string]string {
	var out map[string]string
	put := func(key string, vs []string) {
		if len(vs) == 0 {
			return
		}
		if out == nil {
			out = make(map[string]string)
		}
		out[key] = strings.Join(vs, ",")
	}
	for _, announced := range h.Values("Trailer") {
		for k := range strings.SplitSeq(announced, ",") {
			k = http.CanonicalHeaderKey(strings.TrimSpace(k))
			if k == "" {
				continue
			}
			put(k, h.Values(k))
		}
	}
	for k, vs := range h {
		if !strings.HasPrefix(k, http.TrailerPrefix) {
			continue
		}
		put(strings.TrimPrefix(k, http.TrailerPrefix), vs)
	}
	return out
}
