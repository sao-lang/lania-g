package swagger_test

import (
	bindinghttp "github.com/sao-lang/lania-g/protocol/http/v3/binding"
)

type swaggerUser struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type swaggerErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type swaggerAutoController struct{}

type swaggerCreateUserRequest struct {
	Name   string            `json:"name" description:"user name" example:"alice"`
	Role   string            `json:"role" enum:"admin|guest" default:"guest"`
	Tags   []string          `json:"tags,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
	Secret string            `json:"secret,omitempty" swagger:"-"`
}

func (c *swaggerAutoController) GetUser() error { return nil }

type swaggerQueryArgs struct {
	ID    bindinghttp.Param[string]  `param:"id" required:"true" description:"user id"`
	Trace bindinghttp.Header[string] `header:"X-Trace-ID" description:"trace id"`
}

type swaggerCreateArgs struct {
	Body  bindinghttp.Body[swaggerCreateUserRequest]
	Trace bindinghttp.Header[string] `header:"X-Trace-ID"`
}

type swaggerCreateUserResponse struct {
	ID   int      `json:"id"`
	Tags []string `json:"tags"`
}

type swaggerEnvelope struct {
	Data interface{} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func (c *swaggerAutoController) QueryUser(args swaggerQueryArgs) (swaggerUser, error) {
	return swaggerUser{}, nil
}
func (c *swaggerAutoController) CreateUser(args swaggerCreateArgs) (swaggerCreateUserResponse, error) {
	return swaggerCreateUserResponse{}, nil
}
