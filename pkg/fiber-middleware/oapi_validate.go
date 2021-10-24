// Copyright 2019 DeepMap, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package middleware

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

type fiberContextKey struct{}
type userDataKey struct{}

// This is an Echo middleware function which validates incoming HTTP requests
// to make sure that they conform to the given OAPI 3.0 specification. When
// OAPI validation fails on the request, we return an HTTP/400.

// Create validator middleware from a YAML file path
func OapiValidatorFromYamlFile(path string) (fiber.Handler, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %s", path, err)
	}

	swagger, err := openapi3.NewLoader().LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s as Swagger YAML: %s",
			path, err)
	}
	return OapiRequestValidator(swagger), nil
}

// Create a validator from a swagger object.
func OapiRequestValidator(swagger *openapi3.T) fiber.Handler {
	return OapiRequestValidatorWithOptions(swagger, nil)
}

// Options to customize request validation. These are passed through to
// openapi3filter.
type Options struct {
	Options      openapi3filter.Options
	ParamDecoder openapi3filter.ContentParameterDecoder
	UserData     interface{}
	Skipper      Skipper
}

// Create a validator from a swagger object, with validation options
func OapiRequestValidatorWithOptions(swagger *openapi3.T, options *Options) fiber.Handler {
	router, err := gorillamux.NewRouter(swagger)
	if err != nil {
		panic(err)
	}

	skipper := getSkipperFromOptions(options)
	return func(ctx *fiber.Ctx) error {
		if skipper(ctx) {
			return ctx.Next()
		}

		err := ValidateRequestFromContext(ctx, router, options)
		if err != nil {
			return err
		}

		return ctx.Next()
	}
}

// ValidateRequestFromContext is called from the middleware above and actually does the work
// of validating a request.
func ValidateRequestFromContext(ctx *fiber.Ctx, router routers.Router, options *Options) error {
	req := &http.Request{}
	err := fasthttpadaptor.ConvertRequest(ctx.Context(), req, true)
	if err != nil {
		return err
	}
	route, pathParams, err := router.FindRoute(req)

	// We failed to find a matching route for the request.
	if err != nil {
		switch e := err.(type) {
		case *routers.RouteError:
			// We've got a bad request, the path requested doesn't match
			// either server, or path, or something.
			return fiber.NewError(http.StatusBadRequest, e.Reason)
		default:
			// This should never happen today, but if our upstream code changes,
			// we don't want to crash the server, so handle the unexpected error.
			return fiber.NewError(http.StatusInternalServerError,
				fmt.Sprintf("error validating route: %s", err.Error()))
		}
	}

	validationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
	}

	// Pass the Echo context into the request validator, so that any callbacks
	// which it invokes make it available.
	requestContext := context.WithValue(context.Background(), fiberContextKey{}, ctx)

	if options != nil {
		validationInput.Options = &options.Options
		validationInput.ParamDecoder = options.ParamDecoder
		requestContext = context.WithValue(requestContext, userDataKey{}, options.UserData)
	}

	err = openapi3filter.ValidateRequest(requestContext, validationInput)
	if err != nil {
		switch e := err.(type) {
		case *openapi3filter.RequestError:
			// We've got a bad request
			// Split up the verbose error by lines and return the first one
			// openapi errors seem to be multi-line with a decent message on the first
			errorLines := strings.Split(e.Error(), "\n")
			return &fiber.Error{
				Code:    http.StatusBadRequest,
				Message: errorLines[0],
			}
		case *openapi3filter.SecurityRequirementsError:
			for _, err := range e.Errors {
				httpErr, ok := err.(*fiber.Error)
				if ok {
					return httpErr
				}
			}
			return &fiber.Error{
				Code:    http.StatusForbidden,
				Message: e.Error(),
			}
		default:
			// This should never happen today, but if our upstream code changes,
			// we don't want to crash the server, so handle the unexpected error.
			return &fiber.Error{
				Code:    http.StatusInternalServerError,
				Message: fmt.Sprintf("error validating request: %s", err),
			}
		}
	}
	return nil
}

// Helper function to get the fiber context from within requests. It returns
// nil if not found or wrong type.
func GetFiberContext(c context.Context) *fiber.Ctx {
	iface := c.Value(fiberContextKey{})
	if iface == nil {
		return nil
	}
	eCtx, ok := iface.(*fiber.Ctx)
	if !ok {
		return nil
	}
	return eCtx
}

func GetUserData(c context.Context) interface{} {
	return c.Value(userDataKey{})
}

// Skipper defines a function to skip middleware. Returning true skips processing
// the middleware.
type Skipper func(*fiber.Ctx) bool

// attempt to get the skipper from the options whether it is set or not
func getSkipperFromOptions(options *Options) Skipper {
	if options == nil {
		return DefaultSkipper
	}

	if options.Skipper == nil {
		return DefaultSkipper
	}

	return options.Skipper
}

// DefaultSkipper returns false which processes the middleware.
func DefaultSkipper(*fiber.Ctx) bool {
	return false
}
