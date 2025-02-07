package endpoints

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/krateoplatformops/finops-prometheus-exporter-generic/internal/helpers/kube/httpcall"
	"github.com/krateoplatformops/finops-prometheus-exporter-generic/internal/helpers/kube/secrets"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	finopsdatatypes "github.com/krateoplatformops/finops-data-types/api/v1"
)

type ResolveOptions struct {
	RESTConfig *rest.Config
	API        *finopsdatatypes.API
	AuthNS     string
	Username   string
}

func Resolve(ctx context.Context, opts ResolveOptions) (*httpcall.Endpoint, error) {
	state := opts.RESTConfig.Impersonate
	defer func() {
		opts.RESTConfig.Impersonate = state
	}()

	opts.RESTConfig.Impersonate = rest.ImpersonationConfig{}
	res, err := endpointResolver(opts.RESTConfig, opts.AuthNS, opts.Username)
	if err != nil {
		return &httpcall.Endpoint{}, err
	}

	endpoint, err := res.Do(ctx, opts.API.EndpointRef)
	if err != nil {
		return &httpcall.Endpoint{}, err
	}

	return endpoint, nil
}

func endpointResolver(rc *rest.Config, authNS, username string) (*resolver, error) {
	cli, err := secrets.NewClient(rc)
	if err != nil {
		return nil, err
	}

	return &resolver{
		cli:      cli,
		authNS:   authNS,
		username: username,
	}, nil
}

type resolver struct {
	cli      *secrets.Client
	authNS   string
	username string
}

func (er *resolver) Do(ctx context.Context, ref *finopsdatatypes.ObjectRef) (*httpcall.Endpoint, error) {
	var err error
	res := &httpcall.Endpoint{}
	isInternal := false
	var sec *v1.Secret
	if ref == nil {
		tokenData, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
		if err != nil {
			return &httpcall.Endpoint{}, fmt.Errorf("there has been an error reading the cert-file: %w", err)
		}
		certData, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
		if err != nil {
			return &httpcall.Endpoint{}, fmt.Errorf("there has been an error reading the cert-file: %w", err)
		}
		sec = &v1.Secret{
			Data: map[string][]byte{
				"server-url":                 []byte("https://kubernetes.default.svc"),
				"token":                      tokenData,
				"certificate-authority-data": certData,
				"insecure":                   []byte("true"),
			},
		}
		isInternal = true
	}

	if !isInternal {
		sec, err = er.cli.Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			return res, err
		}
	}

	if v, ok := sec.Data["server-url"]; ok {
		res.ServerURL = string(v)
	} else {
		return res, fmt.Errorf("missed required attribute for endpoint: server-url")
	}

	if v, ok := sec.Data["proxy-url"]; ok {
		res.ProxyURL = string(v)
	}

	if v, ok := sec.Data["token"]; ok {
		res.Token = string(v)
	}

	if v, ok := sec.Data["username"]; ok {
		res.Username = string(v)
	}

	if v, ok := sec.Data["password"]; ok {
		res.Password = string(v)
	}

	if v, ok := sec.Data["certificate-authority-data"]; ok {
		res.CertificateAuthorityData = string(v)
	}

	if v, ok := sec.Data["client-key-data"]; ok {
		res.ClientKeyData = string(v)
	}

	if v, ok := sec.Data["client-certificate-data"]; ok {
		res.ClientCertificateData = string(v)
	}

	if v, ok := sec.Data["debug"]; ok {
		res.Debug, _ = strconv.ParseBool(string(v))
	}

	if v, ok := sec.Data["insecure"]; ok {
		res.Insecure, _ = strconv.ParseBool(string(v))
	}

	return res, nil
}
