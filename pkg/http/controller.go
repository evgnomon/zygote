package http

import (
	"context"
)

type RouteOpt interface {
	Configure(r Router) error
}

// add enum for http methods:
type Method int

const (
	GET Method = iota
	POST
	PUT
	DELETE
	PATCH
)

type Router interface {
	Add(method Method, path string, handler func(Context) error, opts ...RouteOpt) error
}

type Context interface {
	GetUser() (string, error)
	SendUnauthorizedError() error
	SendString(response string) error
	BindBody(b any) error
	SendError(msg string) error
	Send(response any) error
	SendInternalError(msg string, err error) error
	GetRequestContext() context.Context
}

type Controller interface {
	AddEndpoint(prefix string, e Router) error
	Close() error
}
