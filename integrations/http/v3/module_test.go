package http

import (

)

type adminHTTPClient struct{}

func (adminHTTPClient) HTTPClientName() string { return "admin" }

