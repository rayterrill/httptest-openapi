package validator

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	legacyrouter "github.com/getkin/kin-openapi/routers/legacy"
)

type Validator struct {
	Openapi *openapi3.T
}

func (v Validator) Validate(rr *httptest.ResponseRecorder, req *http.Request) error {
	// avoid issues with Host matching, based on Servers
	v.Openapi.Servers = nil

	oaRouter, err := legacyrouter.NewRouter(v.Openapi)
	if err != nil {
		return err
	}

	// ensure that the requested route is found
	route, pathParams, err := oaRouter.FindRoute(req)
	if err != nil {
		return fmt.Errorf("could not find route: %w", err)
	}

	// validate the request
	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
	}
	err = openapi3filter.ValidateRequest(context.Background(), requestValidationInput)
	if err != nil {
		return fmt.Errorf("http request is not valid: %w", err)
	}

	// validate the response
	responseValidationInput := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: requestValidationInput,
		Status:                 rr.Result().StatusCode,
		Header:                 rr.Result().Header,
		Options: &openapi3filter.Options{
			IncludeResponseStatus: true,
		},
	}

	responseValidationInput.SetBodyBytes(rr.Body.Bytes())
	err = openapi3filter.ValidateResponse(context.Background(), responseValidationInput)
	if err != nil {
		return fmt.Errorf("http response is not valid: %w", err)
	}

	// perform additional validation missed from `openapi3filter`
	// https://github.com/getkin/kin-openapi/issues/546
	responseRef := route.Operation.Responses.Get(rr.Result().StatusCode)
	if responseRef == nil {
		responseRef = route.Operation.Responses.Default()
	}

	if responseRef == nil {
		return fmt.Errorf("no response found: %w", err)
	}

	for k, s := range responseRef.Value.Headers {
		h := rr.Result().Header.Get(k)
		if h == "" && s.Value.Required {
			return fmt.Errorf("response missing required response header, %s", k)
		}
		err := s.Value.Schema.Value.VisitJSON(h)
		if err != nil {
			return err
		}
	}

	return nil
}
