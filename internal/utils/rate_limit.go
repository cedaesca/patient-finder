package utils

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"
)

// RateLimitHeaderNames are exposed to clients and reused by CORS config.
var RateLimitHeaderNames = []string{
	"X-RateLimit-Limit",
	"X-RateLimit-Remaining",
	"X-RateLimit-Increment",
	"X-RateLimit-Reset",
	"Retry-After",
}

func DefaultRateLimitResponseHeaders() httprate.ResponseHeaders {
	return httprate.ResponseHeaders{
		Limit:      RateLimitHeaderNames[0],
		Remaining:  RateLimitHeaderNames[1],
		Increment:  RateLimitHeaderNames[2],
		Reset:      RateLimitHeaderNames[3],
		RetryAfter: RateLimitHeaderNames[4],
	}
}

func RetryAfterOnlyRateLimitResponseHeaders() httprate.ResponseHeaders {
	return httprate.ResponseHeaders{
		RetryAfter: RateLimitHeaderNames[4],
	}
}

func LimitByRealIPWithHeaders(requestLimit int, windowLength time.Duration, options ...httprate.Option) func(next http.Handler) http.Handler {
	baseOptions := []httprate.Option{
		httprate.WithKeyByRealIP(),
		httprate.WithResponseHeaders(DefaultRateLimitResponseHeaders()),
	}

	return httprate.Limit(requestLimit, windowLength, append(baseOptions, options...)...)
}

func LimitByRealIPWithoutHeaders(requestLimit int, windowLength time.Duration, options ...httprate.Option) func(next http.Handler) http.Handler {
	baseOptions := []httprate.Option{
		httprate.WithKeyByRealIP(),
		httprate.WithResponseHeaders(httprate.ResponseHeaders{}),
	}

	return httprate.Limit(requestLimit, windowLength, append(baseOptions, options...)...)
}

func LimitByRealIPWithRetryAfterOnly(requestLimit int, windowLength time.Duration, options ...httprate.Option) func(next http.Handler) http.Handler {
	baseOptions := []httprate.Option{
		httprate.WithKeyByRealIP(),
		httprate.WithResponseHeaders(RetryAfterOnlyRateLimitResponseHeaders()),
	}

	return httprate.Limit(requestLimit, windowLength, append(baseOptions, options...)...)
}

func LimitByKeyFuncsWithHeaders(requestLimit int, windowLength time.Duration, keyFuncs ...httprate.KeyFunc) func(next http.Handler) http.Handler {
	baseOptions := []httprate.Option{
		httprate.WithResponseHeaders(DefaultRateLimitResponseHeaders()),
	}

	if len(keyFuncs) > 0 {
		baseOptions = append(baseOptions, httprate.WithKeyFuncs(keyFuncs...))
	}

	return httprate.Limit(requestLimit, windowLength, baseOptions...)
}

func LimitByKeyFuncsWithRetryAfterOnly(requestLimit int, windowLength time.Duration, keyFuncs ...httprate.KeyFunc) func(next http.Handler) http.Handler {
	baseOptions := []httprate.Option{
		httprate.WithResponseHeaders(RetryAfterOnlyRateLimitResponseHeaders()),
	}

	if len(keyFuncs) > 0 {
		baseOptions = append(baseOptions, httprate.WithKeyFuncs(keyFuncs...))
	}

	return httprate.Limit(requestLimit, windowLength, baseOptions...)
}

func LimitByRequestKeyWithHeaders(requestLimit int, windowLength time.Duration, keyExtractor func(*http.Request) (string, bool), options ...httprate.Option) func(next http.Handler) http.Handler {
	baseOptions := []httprate.Option{
		httprate.WithKeyFuncs(requestKeyOrRealIP(keyExtractor)),
		httprate.WithResponseHeaders(DefaultRateLimitResponseHeaders()),
	}

	return httprate.Limit(requestLimit, windowLength, append(baseOptions, options...)...)
}

func LimitByRequestKeyWithRetryAfterOnly(requestLimit int, windowLength time.Duration, keyExtractor func(*http.Request) (string, bool), options ...httprate.Option) func(next http.Handler) http.Handler {
	baseOptions := []httprate.Option{
		httprate.WithKeyFuncs(requestKeyOrRealIP(keyExtractor)),
		httprate.WithResponseHeaders(RetryAfterOnlyRateLimitResponseHeaders()),
	}

	return httprate.Limit(requestLimit, windowLength, append(baseOptions, options...)...)
}

func requestKeyOrRealIP(keyExtractor func(*http.Request) (string, bool)) httprate.KeyFunc {
	return func(r *http.Request) (string, error) {
		if keyExtractor != nil {
			if key, ok := keyExtractor(r); ok && key != "" {
				return key, nil
			}
		}

		return httprate.KeyByRealIP(r)
	}
}
