package validation

import (
	"encoding/json"
	"errors"
	app "github.com/cetrictech/go-platform-kit/errors"
	"io"
	"net/http"
)

// ApplicationError converts field failures into approved public validation details, preserving the original cause.
func ApplicationError(err error) *app.AppError {
	if err == nil {
		return nil
	}
	details := map[string]any{}
	var fields Errors
	if errors.As(err, &fields) {
		details["fields"] = fields
	}
	return app.Wrap(err, "Please correct the highlighted fields.", app.WithCode("VALIDATION_ERROR"), app.WithCategory(app.Validation), app.WithSeverity(app.Warning), app.WithHTTPStatus(http.StatusUnprocessableEntity), app.WithDetails(details))
}

// DecodeJSON consumes and closes one bounded JSON request body, rejects unknown fields/trailing documents, then validates dst.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	if r == nil || r.Body == nil {
		return app.New("A JSON request body is required.", app.WithCode("INVALID_JSON"), app.WithCategory(app.Validation), app.WithHTTPStatus(http.StatusBadRequest))
	}
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	body := http.MaxBytesReader(w, r.Body, maxBytes)
	defer body.Close()
	d := json.NewDecoder(body)
	d.DisallowUnknownFields()
	if e := d.Decode(dst); e != nil {
		return decodeError(e)
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		if e == nil {
			e = errors.New("multiple JSON documents")
		}
		return decodeError(e)
	}
	return validationError(Struct(dst))
}
func validationError(e error) error {
	if e == nil {
		return nil
	}
	return ApplicationError(e)
}
func decodeError(e error) error {
	status := http.StatusBadRequest
	var large *http.MaxBytesError
	if errors.As(e, &large) {
		status = http.StatusRequestEntityTooLarge
	}
	return app.Wrap(e, "The request body is not valid JSON.", app.WithCode("INVALID_JSON"), app.WithCategory(app.Validation), app.WithHTTPStatus(status))
}
