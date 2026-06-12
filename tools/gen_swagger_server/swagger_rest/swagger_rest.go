package swagger_rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

func BadRequest(code, msg string, args ...any) *ClientError {
	return NewClientError(http.StatusBadRequest, code, msg, args...)
}

func NewClientError(status int, code, msg string, args ...any) *ClientError {
	// FIXME This should ideally prevent `StatusNotFound` being passed as
	// `status` because the `code` and `msg` will be overridden by `SendResp` in
	// this case.
	return &ClientError{
		status: status,
		code:   code,
		msg:    fmt.Sprintf(msg, args...),
	}
}

type ClientError struct {
	status  int
	code    string
	msg     string
	context map[string]any
}

func BadRequestWithContext(code string, context map[string]any, msg string, args ...any) *ClientError {
	return NewClientErrorWithContext(http.StatusBadRequest, code, context, msg, args...)
}

func NewClientErrorWithContext(status int, code string, context map[string]any, msg string, args ...any) *ClientError {
	// FIXME This should ideally prevent `StatusNotFound` being passed as
	// `status` because the `code` and `msg` will be overridden by `SendResp` in
	// this case.
	return &ClientError{
		status:  status,
		code:    code,
		msg:     fmt.Sprintf(msg, args...),
		context: context,
	}
}

func NotFound(msg string, args ...any) *ClientError {
	// FIXME The `code` parameter isn't set here because it's automatically
	// populated by `SendResp`.
	return NewClientError(http.StatusNotFound, "", msg, args...)
}

func (e *ClientError) Error() string {
	if e.status == http.StatusNotFound {
		return fmt.Sprintf("not found (%d): %s", e.status, e.msg)
	}
	return fmt.Sprintf("client error (%d): '%s': %s", e.status, e.code, e.msg)
}

func IsClientError(e error) bool {
	ce := &ClientError{}
	return errors.As(e, &ce)
}

// `SendResp` sends `respBody` with `successStatus` through `w` if `respErr` is
// `nil`, otherwise it will send a client error or server error depending on
// whether `respErr` is a `ClientError` or not.
//
// If `SendResp` returns an error then it indicates that either an error
// occurred while writing the response or after this point, meaning that after
// calling `SendResp` no further writes to the client should occur (if an error
// occured before sending the response then we would need to attempt to send an
// error at a higher level).
func SendResp(
	w http.ResponseWriter,
	successStatus int,
	respBody any,
	respErr error,
) error {
	var resp *response

	if respErr == nil {
		if respBody == nil {
			// TODO `successStatus` should only ever be
			// `http.StatusNoContent` in this branch. An approach to
			// enforce this more comprehensively should ideally be
			// implemented when time allows.
			resp = &response{status: successStatus}
		} else {
			resp = &response{
				status: successStatus,
				body:   respBody,
			}
		}
	} else {
		ce := &ClientError{}
		if errors.As(respErr, &ce) {
			// TODO Confirm that `ce.status` is a value that this endpoint has
			// defined and issue an error if it isn't.
			if ce.status == http.StatusNotFound {
				resp = &response{
					status: ce.status,
					body: &errBody{
						Code: "not_found",
						Msg:  "no resource was found under the provided identifier",
					},
				}
			} else {
				resp = &response{
					status: ce.status,
					body: &errBody{
						Code:    ce.code,
						Msg:     ce.msg,
						Context: ce.context,
					},
				}
			}
		} else {
			// Note that we don't handle the logging, etc. of the server error
			// here, we delegate that responsibility to the caller.
			resp = &response{
				status: http.StatusInternalServerError,
				body: &errBody{
					Code: "server_error",
					Msg:  "internal server error",
				},
			}
		}
	}

	if resp.status == http.StatusNoContent {
		w.WriteHeader(resp.status)
		return nil
	}

	body, marshalErr := json.MarshalIndent(resp.body, prefix, indent)
	if marshalErr != nil {
		body = []byte(`{
			"code": "server_error",
			"message": "internal server error"
		}`)
		// We handle `marshalErr` after we write the response so that
		// all errors either occur while writing a response or after a
		// response has been written. This means that the server error
		// response doesn't need to be sent at a higher level for this
		// case.
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.status)
	_, err := w.Write(body)
	if err != nil {
		return fmt.Errorf("couldn't write response ('%s'): %w", string(body), err)
	}
	if marshalErr != nil {
		return fmt.Errorf("couldn't encode response ('%v'): %w", resp.body, marshalErr)
	}

	return nil
}

const (
	indent = "\t"
	prefix = ""
)

type response struct {
	status int
	body   any
}

type errBody struct {
	Code    string         `json:"code"`
	Msg     string         `json:"message"`
	Context map[string]any `json:"context,omitempty"`
}

type DictGetter interface {
	Get(key string) (string, bool)
}

func HandlePathString(dictGetter DictGetter, name string) (string, error) {
	raw, ok := dictGetter.Get(name)
	if !ok {
		br := BadRequest(
			name+"_missing",
			"no value was passed for '%s'",
			name,
		)
		return "", br
	}
	return raw, nil
}

func HandlePathInt(dictGetter DictGetter, name string) (int, error) {
	raw, ok := dictGetter.Get(name)
	if !ok {
		br := BadRequest(
			name+"_missing",
			"no value was passed for '%s'",
			name,
		)
		return 0, br
	}

	val, err := strconv.Atoi(raw)
	if err != nil {
		br := BadRequest(
			"invalid_"+name,
			"couldn't parse '%s' as an integer",
			name,
		)
		return 0, br
	}

	return val, nil
}

func HandleQueryOptionalString(urlVals url.Values, name string) (*string, error) {
	vals, ok := urlVals[name]
	if !ok {
		return nil, nil
	}

	if len(vals) > 1 {
		br := BadRequest(
			"multiple_"+name,
			"multiple values were passed for '%s'",
			name,
		)
		return nil, br
	}

	return &vals[0], nil
}

func HandleQueryOptionalInt(urlVals url.Values, name string) (*int, error) {
	vals, ok := urlVals[name]
	if !ok {
		return nil, nil
	}

	if len(vals) > 1 {
		br := BadRequest(
			"multiple_"+name,
			"multiple values were passed for '%s'",
			name,
		)
		return nil, br
	}

	val, err := strconv.Atoi(vals[0])
	if err != nil {
		br := BadRequest(
			"invalid_"+name,
			"couldn't parse '%s' as an integer",
			name,
		)
		return nil, br
	}

	return &val, nil
}

func HandleQueryOptionalFloat64(urlVals url.Values, name string) (*float64, error) {
	vals, ok := urlVals[name]
	if !ok {
		return nil, nil
	}

	if len(vals) > 1 {
		br := BadRequest(
			"multiple_"+name,
			"multiple values were passed for '%s'",
			name,
		)
		return nil, br
	}

	val, err := strconv.ParseFloat(vals[0], 64)
	if err != nil {
		br := BadRequest(
			"invalid_"+name,
			"couldn't parse '%s' as a float",
			name,
		)
		return nil, br
	}

	return &val, nil
}

func HandleQueryRequiredString(urlVals url.Values, name string) (string, error) {
	vals, ok := urlVals[name]
	if !ok {
		br := BadRequest(
			name+"_missing",
			"no value was passed for '%s'",
			name,
		)
		return "", br
	}

	if len(vals) > 1 {
		br := BadRequest(
			"multiple_"+name,
			"multiple values were passed for '%s'",
			name,
		)
		return "", br
	}

	return vals[0], nil
}

func HandleQueryRequiredInt(urlVals url.Values, name string) (int, error) {
	vals, ok := urlVals[name]
	if !ok {
		br := BadRequest(
			name+"_missing",
			"no value was passed for '%s'",
			name,
		)
		return 0, br
	}

	if len(vals) > 1 {
		br := BadRequest(
			"multiple_"+name,
			"multiple values were passed for '%s'",
			name,
		)
		return 0, br
	}

	val, err := strconv.Atoi(vals[0])
	if err != nil {
		br := BadRequest(
			"invalid_"+name,
			"couldn't parse '%s' as an integer",
			name,
		)
		return 0, br
	}

	return val, nil
}

func HandleQueryOptionalBool(urlVals url.Values, name string) (*bool, error) {
	vals, ok := urlVals[name]
	if !ok {
		return nil, nil
	}

	if len(vals) > 1 {
		br := BadRequest(
			"multiple_"+name,
			"multiple values were passed for '%s'",
			name,
		)
		return nil, br
	}

	val, err := strconv.ParseBool(vals[0])
	if err != nil {
		br := BadRequest(
			"invalid_"+name,
			"couldn't parse '%s' as a boolean",
			name,
		)
		return nil, br
	}

	return &val, nil
}

type MissingParamError struct {
	Name string
}

func (e *MissingParamError) Error() string {
	return fmt.Sprintf("required parameter '%s' wasn't provided", e.Name)
}

type InvalidParamError struct {
	Name  string
	Value string
}

func (e *InvalidParamError) Error() string {
	return fmt.Sprintf(
		"invalid value ('%s') given for parameter '%s'",
		e.Name,
		e.Value,
	)
}

func NewHandleEndpointError(operationID, msg string, err error) error {
	return &HandleEndpointError{
		OperationID: operationID,
		Msg:         msg,
		Err:         err,
	}
}

// `HandleEndpointErrors` are returned from the `Handle` methods for Swagger
// endpoints. At the time this error is returned, headers will already have been
// written and an attempt to write the response body will either have started or
// completed. As such, error handling for this error should not attempt to write
// a response.
type HandleEndpointError struct {
	OperationID string
	Msg         string
	Err         error
}

func (e *HandleEndpointError) Error() string {
	return fmt.Sprintf("'%s' failed: %s: %v", e.OperationID, e.Msg, e.Err)
}

func (e *HandleEndpointError) Unwrap() error {
	return e.Err
}
