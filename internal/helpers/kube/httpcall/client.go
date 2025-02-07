package httpcall

import (
	"fmt"
	"net/http"
)

func HTTPClientForEndpoint(authn *Endpoint) (*http.Client, error) {
	rt, err := tlsConfigFor(authn)
	if err != nil {
		return &http.Client{
			Transport: defaultTransport(),
		}, err
	}

	if authn.Debug {
		rt = &debuggingRoundTripper{
			delegatedRoundTripper: rt,
		}
	}

	// Set authentication wrappers
	switch {
	case authn.HasBasicAuth() && authn.HasTokenAuth():
		return nil, fmt.Errorf("username/password or bearer token may be set, but not both")

	case authn.HasTokenAuth():
		rt = &bearerAuthRoundTripper{
			bearer: authn.Token,
			rt:     rt,
		}

	case authn.HasBasicAuth():
		rt = &basicAuthRoundTripper{
			username: authn.Username,
			password: authn.Password,
			rt:       rt,
		}
	}

	return &http.Client{Transport: rt}, nil
}
