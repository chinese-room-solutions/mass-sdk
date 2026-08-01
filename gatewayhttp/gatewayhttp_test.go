package gatewayhttp

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlattenResponseHeaders(t *testing.T) {
	tests := []struct {
		name string
		in   http.Header
		want map[string]string
	}{
		{
			name: "plain headers pass through",
			in: http.Header{
				"Content-Type": []string{"application/json"},
				"X-Foo":        []string{"bar"},
			},
			want: map[string]string{
				"Content-Type": "application/json",
				"X-Foo":        "bar",
			},
		},
		{
			name: "multi-value joined with comma",
			in: http.Header{
				"Set-Cookie": []string{"a=1", "b=2"},
			},
			want: map[string]string{
				"Set-Cookie": "a=1,b=2",
			},
		},
		{
			name: "announced trailer key excluded from headers",
			in: http.Header{
				"Content-Type":  []string{"application/grpc"},
				"Trailer":       []string{"Grpc-Status, Grpc-Message"},
				"Grpc-Status":   []string{"0"},
				"Grpc-Message":  []string{""},
				"Other-Header":  []string{"keep"},
			},
			want: map[string]string{
				"Content-Type": "application/grpc",
				"Other-Header": "keep",
			},
		},
		{
			name: "TrailerPrefix entries excluded from headers",
			in: http.Header{
				"Content-Type":             []string{"text/plain"},
				http.TrailerPrefix + "End": []string{"done"},
			},
			want: map[string]string{
				"Content-Type": "text/plain",
			},
		},
		{
			// Same multi-Add gotcha as extractTrailers: with two
			// separate Trailer entries the announced set must include
			// both names so both stay out of the headers map.
			name: "multi-Add Trailer excludes all announced names",
			in: http.Header{
				"Content-Type": []string{"application/grpc"},
				"Trailer":      []string{"Grpc-Status", "Grpc-Message"},
				"Grpc-Status":  []string{"0"},
				"Grpc-Message": []string{""},
				"Keep-Me":      []string{"yes"},
			},
			want: map[string]string{
				"Content-Type": "application/grpc",
				"Keep-Me":      "yes",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flattenResponseHeaders(tt.in)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestExtractTrailers(t *testing.T) {
	tests := []struct {
		name string
		in   http.Header
		want map[string]string
	}{
		{
			name: "no trailers returns nil",
			in: http.Header{
				"Content-Type": []string{"text/plain"},
			},
			want: nil,
		},
		{
			name: "announced trailers picked up",
			in: http.Header{
				"Trailer":      []string{"Grpc-Status, Grpc-Message"},
				"Grpc-Status":  []string{"0"},
				"Grpc-Message": []string{""},
			},
			want: map[string]string{
				"Grpc-Status":  "0",
				"Grpc-Message": "",
			},
		},
		{
			name: "TrailerPrefix late-bound trailers picked up",
			in: http.Header{
				http.TrailerPrefix + "End-Token": []string{"abc"},
			},
			want: map[string]string{
				"End-Token": "abc",
			},
		},
		{
			name: "mixed announced and late-bound",
			in: http.Header{
				"Trailer":                       []string{"X-Pre"},
				"X-Pre":                         []string{"declared"},
				http.TrailerPrefix + "X-Late":   []string{"runtime"},
			},
			want: map[string]string{
				"X-Pre":  "declared",
				"X-Late": "runtime",
			},
		},
		{
			// gRPC issues two separate h.Add("Trailer", …) calls →
			// two slice entries under "Trailer". h.Get() returns
			// only the first, so older code silently dropped
			// Grpc-Message and clients saw empty desc.
			name: "multi-Add Trailer announces both names",
			in: http.Header{
				"Trailer":      []string{"Grpc-Status", "Grpc-Message"},
				"Grpc-Status":  []string{"3"},
				"Grpc-Message": []string{"model is required"},
			},
			want: map[string]string{
				"Grpc-Status":  "3",
				"Grpc-Message": "model is required",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTrailers(tt.in)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIsGRPCRequest(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "plain grpc", contentType: "application/grpc", want: true},
		{name: "grpc with proto subtype", contentType: "application/grpc+proto", want: true},
		{name: "grpc-web also matches prefix", contentType: "application/grpc-web", want: true},
		{name: "json is not grpc", contentType: "application/json", want: false},
		{name: "empty", contentType: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{Header: http.Header{}}
			if tt.contentType != "" {
				r.Header.Set("Content-Type", tt.contentType)
			}
			require.Equal(t, tt.want, isGRPCRequest(r))
		})
	}
}
