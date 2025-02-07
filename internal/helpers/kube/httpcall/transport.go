package httpcall

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

func tlsConfigFor(e *Endpoint) (http.RoundTripper, error) {
	res := defaultTransport()

	if e.ProxyURL != "" {
		u, err := parseProxyURL(e.ProxyURL)
		if err != nil {
			return nil, err
		}

		res.Proxy = http.ProxyURL(u)
	}

	if !e.HasCertAuth() {
		if e.Insecure {
			res.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
		return res, nil
	}

	certData, err := base64.StdEncoding.DecodeString(e.ClientCertificateData)
	if err != nil {
		return nil, fmt.Errorf("unable to decode client certificate data")
	}

	keyData, err := base64.StdEncoding.DecodeString(e.ClientKeyData)
	if err != nil {
		return nil, fmt.Errorf("unable to decode client key data")
	}

	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return res, err
	}

	caCertPool := x509.NewCertPool()

	if len(e.CertificateAuthorityData) > 0 {
		caData, err := base64.StdEncoding.DecodeString(e.CertificateAuthorityData)
		if err != nil {
			return nil, fmt.Errorf("unable to decode certificate authority data")
		}

		caCertPool.AppendCertsFromPEM(caData)
	}

	tlsConfig := &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{cert},
	}

	res.TLSClientConfig = tlsConfig
	return res, nil
}

func defaultTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func parseProxyURL(proxyURL string) (*url.URL, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("could not parse: %v", proxyURL)
	}

	switch u.Scheme {
	case "http", "https", "socks5":
	default:
		return nil, fmt.Errorf("unsupported scheme %q, must be http, https, or socks5", u.Scheme)
	}
	return u, nil
}

type debuggingRoundTripper struct {
	delegatedRoundTripper http.RoundTripper
}

func (rt *debuggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	b, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		return nil, err
	}
	os.Stderr.Write(b)
	os.Stderr.WriteString("\n\n")

	resp, err := rt.delegatedRoundTripper.RoundTrip(req)

	// If an error was returned, dump it to os.Stderr.
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return resp, err
	}

	b, err = httputil.DumpResponse(resp, req.URL.Query().Get("watch") != "true")
	if err != nil {
		return nil, err
	}
	os.Stderr.Write(b)
	os.Stderr.Write([]byte{'\n'})

	return resp, err
}

type basicAuthRoundTripper struct {
	username string
	password string `datapolicy:"password"`
	rt       http.RoundTripper
}

func (rt *basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("Authorization")) != 0 {
		return rt.rt.RoundTrip(req)
	}
	req = cloneRequest(req)
	req.SetBasicAuth(rt.username, rt.password)
	return rt.rt.RoundTrip(req)
}

type bearerAuthRoundTripper struct {
	bearer string
	rt     http.RoundTripper
}

func (rt *bearerAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(req.Header.Get("Authorization")) != 0 {
		return rt.rt.RoundTrip(req)
	}

	req = cloneRequest(req)
	token := rt.bearer

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return rt.rt.RoundTrip(req)
}
