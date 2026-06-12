package swagger_rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	rest_api_url "github.com/trustap/rest_api/pkg/url"
)

func NewURL(u *url.URL) *URL {
	return &URL{u}
}

type URL struct {
	*url.URL
}

func (u *URL) UnmarshalJSON(p []byte) error {
	var rawURL string
	err := json.Unmarshal(p, &rawURL)
	if err != nil {
		return fmt.Errorf("couldn't unmarshal JSON string: %w", err)
	}

	baseURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("couldn't parse URL '%s': %w", rawURL, err)
	}
	*u = *NewURL(baseURL)

	return nil
}

func (u *URL) MarshalJSON() ([]byte, error) {
	s := fmt.Sprintf(`"%s"`, u.String())
	return []byte(s), nil
}

func NewAbsoluteURL(u *rest_api_url.Absolute) *AbsoluteURL {
	return &AbsoluteURL{u}
}

type AbsoluteURL struct {
	*rest_api_url.Absolute
}

func (u *AbsoluteURL) UnmarshalJSON(p []byte) error {
	var baseURL *URL
	err := json.Unmarshal(p, &baseURL)
	if err != nil {
		return fmt.Errorf("couldn't unmarshal JSON URL: %w", err)
	}

	absURL, err := rest_api_url.NewAbsolute(baseURL.URL)
	if err != nil {
		if errors.Is(err, rest_api_url.ErrMissingScheme) {
			return &MissingSubcomponentError{
				Subcomponent: SubcomponentScheme,
				URL:          baseURL.URL,
			}
		}
		if errors.Is(err, rest_api_url.ErrMissingHost) {
			return &MissingSubcomponentError{
				Subcomponent: SubcomponentHost,
				URL:          baseURL.URL,
			}
		}
		return fmt.Errorf("couldn't construct absolute URL from '%s': %w", baseURL, err)
	}
	*u = *NewAbsoluteURL(absURL)

	return nil
}

func (u *AbsoluteURL) MarshalJSON() ([]byte, error) {
	s := fmt.Sprintf(`"%s"`, u.String())
	return []byte(s), nil
}

type MissingSubcomponentError struct {
	Subcomponent Subcomponent
	URL          *url.URL
}

func (e *MissingSubcomponentError) Error() string {
	return fmt.Sprintf("'%s' is missing %s subcomponent", e.URL.String(), e.Subcomponent)
}

type Subcomponent string

const (
	SubcomponentScheme Subcomponent = "scheme"
	SubcomponentHost   Subcomponent = "host"
)
