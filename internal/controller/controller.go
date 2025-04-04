package controller

import "github.com/labstack/echo/v4"

type Controller interface {
	AddEndpoint(prefix string, e *echo.Echo) error
	Close() error
}
