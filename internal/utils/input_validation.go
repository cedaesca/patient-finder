package utils

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
)

var bcplocaleRe = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)

func NewValidator() *validator.Validate {
	validate := validator.New()

	validate.RegisterValidation("locale", func(fl validator.FieldLevel) bool {
		return bcplocaleRe.MatchString(fl.Field().String())
	})

	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "" || name == "-" {
			return fld.Name
		}
		return name
	})
	return validate
}

func ExtractValidationErrors(err error) (map[string]string, bool) {
	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return nil, false
	}

	errors := make(map[string]string, len(validationErrors))
	for _, fieldErr := range validationErrors {
		errors[fieldErr.Field()] = validationErrorMessage(fieldErr)
	}

	return errors, true
}

func HandleValidationErrorHttpResponse(w http.ResponseWriter, err error) {
	errorDetails, ok := ExtractValidationErrors(err)
	if ok {
		WriteJSON(w, http.StatusBadRequest, Envelope{
			"message": "validation error",
			"errors":  errorDetails,
		})

		return
	}

	WriteJSON(w, http.StatusInternalServerError, Envelope{"message": "internal server error"})
}

func validationErrorMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", fe.Field())
	case "email":
		return fmt.Sprintf("%s must be a valid email address", fe.Field())
	case "min":
		return fmt.Sprintf("%s must be at least %s characters", fe.Field(), fe.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s characters", fe.Field(), fe.Param())
	case "locale":
		return fmt.Sprintf("%s must be a valid locale (BCP 47)", fe.Field())
	case "valid_regex":
		return fmt.Sprintf("%s must be a valid regex", fe.Field())
	case "min_lte_max":
		return fmt.Sprintf("%s must be greater than or equal to min", fe.Field())
	case "required_when_exclusive_min":
		return fmt.Sprintf("%s is required when exclusive_min is true", fe.Field())
	case "required_when_exclusive_max":
		return fmt.Sprintf("%s is required when exclusive_max is true", fe.Field())
	case "min_length_lte_max_length":
		return fmt.Sprintf("%s must be greater than or equal to min_length", fe.Field())
	case "scale_lte_precision":
		return fmt.Sprintf("%s must be less than or equal to precision", fe.Field())
	case "required_for_patch":
		return "at least one updatable field is required"
	default:
		return fmt.Sprintf("%s is invalid", fe.Field())
	}
}
