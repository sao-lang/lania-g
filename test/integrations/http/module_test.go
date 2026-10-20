package http_test

import ()

type adminHTTPClient struct{}

func (adminHTTPClient) HTTPClientName() string { return "admin" }
