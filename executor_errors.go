package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/bearer"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/qoderstream"
)

type executorFailure struct {
	code    string
	message string
	status  int
	cause   error
}

func (e *executorFailure) Error() string { return e.message }
func (e *executorFailure) Unwrap() error { return e.cause }

func classifyExecutorError(err error) *executorFailure {
	var failure *executorFailure
	if errors.As(err, &failure) {
		return failure
	}
	var streamBiz *qodertransport.StreamBusinessError
	if errors.As(err, &streamBiz) {
		return &executorFailure{
			code:    streamBiz.Category,
			message: streamBiz.Error(),
			status:  502, // opaque upstream; do not expose or mutate quota
			cause:   err,
		}
	}
	var bearerHTTP *bearer.HTTPError
	if errors.As(err, &bearerHTTP) {
		return httpFailure(bearerHTTP.StatusCode, bearerHTTP.RetryAfterSec, bearerHTTP.Error(), err)
	}
	var cosyHTTP *cosy.HTTPError
	if errors.As(err, &cosyHTTP) {
		return httpFailure(cosyHTTP.StatusCode, cosyHTTP.RetryAfterSec, cosyHTTP.Error(), err)
	}
	if errors.Is(err, context.Canceled) {
		return &executorFailure{code: "client_canceled", message: "request canceled", status: 499, cause: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &executorFailure{code: "upstream_timeout", message: "upstream timed out", status: http.StatusGatewayTimeout, cause: err}
	}
	return &executorFailure{code: "upstream_error", message: err.Error(), status: http.StatusBadGateway, cause: err}
}

// httpFailure maps a transport HTTP error to a safe outward code and appends
// any parsed Retry-After wait to the message.
func httpFailure(status, retrySec int, msg string, cause error) *executorFailure {
	if retrySec > 0 {
		msg += " retry_after=" + strconv.Itoa(retrySec) + "s"
	}
	return &executorFailure{
		code:    qoderstream.StatusCategory(status),
		message: msg,
		status:  status,
		cause:   cause,
	}
}
