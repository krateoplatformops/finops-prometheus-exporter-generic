package httpcall

type Endpoint struct {
	ServerURL                string
	ProxyURL                 string
	CertificateAuthorityData string
	ClientCertificateData    string
	ClientKeyData            string
	Token                    string
	Username                 string
	Password                 string
	Insecure                 bool
	Debug                    bool
}

// HasCA returns whether the configuration has a certificate authority or not.
func (ep *Endpoint) HasCA() bool {
	return len(ep.CertificateAuthorityData) > 0
}

// HasBasicAuth returns whether the configuration has basic authentication or not.
func (ep *Endpoint) HasBasicAuth() bool {
	return len(ep.Password) != 0
}

// HasTokenAuth returns whether the configuration has token authentication or not.
func (ep *Endpoint) HasTokenAuth() bool {
	return len(ep.Token) != 0
}

// HasCertAuth returns whether the configuration has certificate authentication or not.
func (ep *Endpoint) HasCertAuth() bool {
	return len(ep.ClientCertificateData) != 0 && len(ep.ClientKeyData) != 0
}
