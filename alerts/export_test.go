package alerts

import "crypto/x509"

// TrustForTest makes e trust a test server's self-signed certificate.
func (e *Email) TrustForTest(p *x509.CertPool) { e.rootCAs = p }
