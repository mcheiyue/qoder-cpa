package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/bearer"
	"github.com/mcheiyue/qoder-cpa/internal/qodertransport/cosy"
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
	var bearerHTTP *bearer.HTTPError
	if errors.As(err, &bearerHTTP) {
		return &executorFailure{code: "upstream_error", message: bearerHTTP.Error(), status: bearerHTTP.StatusCode, cause: err}
	}
	var cosyHTTP *cosy.HTTPError
	if errors.As(err, &cosyHTTP) {
		return &executorFailure{code: "upstream_error", message: cosyHTTP.Error(), status: cosyHTTP.StatusCode, cause: err}
	}
	if errors.Is(err, context.Canceled) {
		return &executorFailure{code: "client_canceled", message: "request canceled", status: 499, cause: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &executorFailure{code: "upstream_timeout", message: "upstream timed out", status: http.StatusGatewayTimeout, cause: err}
	}
	return &executorFailure{code: "upstream_error", message: err.Error(), status: http.StatusBadGateway, cause: err}
}
