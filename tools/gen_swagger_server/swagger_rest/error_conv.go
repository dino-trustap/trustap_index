package swagger_rest

import (
	"errors"
	"fmt"
	"strings"

	rest_api_json "github.com/trustap/rest_api/pkg/json"
)

func JSONDecodeErrorToClientError(err error) (*ClientError, bool) {
	var uErr *rest_api_json.UnmarshalError
	if !errors.As(err, &uErr) {
		return nil, false
	}

	subErrs := []*subError{}

	for _, err := range uErr.Errors {
		key := "body"
		path := "$"
		if e := (&rest_api_json.PropertyError{}); errors.As(err, &e) {
			path = e.Path
			key = strings.ReplaceAll(path, ".", "_")
			err = e.Err
		}

		if errors.Is(err, rest_api_json.ErrMissingRequiredField) {
			se := &subError{
				code: "missing_" + key,
				msg:  fmt.Sprintf("required parameter '%s' wasn't provided", path),
			}
			subErrs = append(subErrs, se)
			continue
		}

		if e := (&MissingSubcomponentError{}); errors.As(err, &e) {
			subc := string(e.Subcomponent)
			se := &subError{
				code: "no_" + subc + "_in_" + key,
				msg:  fmt.Sprintf("'%s' URL ('%s') doesn't contain %s", path, e.URL, subc),
			}
			subErrs = append(subErrs, se)
			continue
		}

		var vErr *rest_api_json.ValidationError
		if errors.As(err, &vErr) {
			se := &subError{
				code: "invalid_" + key,
				msg:  fmt.Sprintf("'%s' failed validation", path),
			}

			if vErr.Kind == rest_api_json.ValidationKindMaxLength {
				se = &subError{
					code: key + "_too_long",
					msg: fmt.Sprintf(
						"parameter '%s' can't be more than %d characters",
						path,
						vErr.Target,
					),
				}
			}

			if vErr.Kind == rest_api_json.ValidationKindMinLength {
				if vErr.Target == 1 {
					se = &subError{
						code: "empty_" + key,
						msg:  fmt.Sprintf("parameter '%s' can't be empty", path),
					}
				} else {
					se = &subError{
						code: key + "_too_short",
						msg: fmt.Sprintf(
							"parameter '%s' can't be less than %d characters",
							path,
							vErr.Target,
						),
					}
				}
			}
			subErrs = append(subErrs, se)
			continue
		}

		var tErr *rest_api_json.TypeError
		if errors.As(err, &tErr) {
			se := &subError{
				code: "incorrect_" + key + "_type",
				msg: fmt.Sprintf(
					// TODO Output the value in a
					// representation that shows its type.
					"incorrect type passed for '%s'",
					path,
				),
			}
			subErrs = append(subErrs, se)
			continue
		}
	}

	if len(subErrs) == 0 {
		return nil, false
	}

	context := map[string]any{}
	for _, se := range subErrs {
		context[se.code] = se.msg
	}

	clientError := BadRequestWithContext(subErrs[0].code, context, "%s", subErrs[0].msg)

	return clientError, true
}

type subError struct {
	code string
	msg  string
}
