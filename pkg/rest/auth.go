package rest

import (
	"encoding/base64"
	"net/http"
)

type Authenticator interface {
	Apply(req *http.Request)
}

type BasicAuth struct {
	User     string
	Password string
}

func (b BasicAuth) Apply(req *http.Request) {
	if b.User != "" {
		req.Header.Set("X-Trino-User", b.User)
	}

	if b.User != "" || b.Password != "" {
		creds := b.User + ":" + b.Password
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(creds)))
	}
}

type BearerAuth struct {
	User  string
	Token string
}

func (b BearerAuth) Apply(req *http.Request) {
	if b.User != "" {
		req.Header.Set("X-Trino-User", b.User)
	}

	if b.Token != "" {
		req.Header.Set("Authorization", "Bearer "+b.Token)
	}
}

type NoAuth struct {
	User string
}

func (n NoAuth) Apply(req *http.Request) {
	if n.User != "" {
		req.Header.Set("X-Trino-User", n.User)
	}
}
