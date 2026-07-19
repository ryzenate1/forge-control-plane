package http

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

// Validator is a global validator instance
var validate = validator.New()

// ValidationErrors represents a collection of validation errors
type ValidationErrors struct {
	Errors map[string]string `json:"errors"`
}

// Error implements the error interface
func (ve ValidationErrors) Error() string {
	var errs []string
	for field, msg := range ve.Errors {
		errs = append(errs, fmt.Sprintf("%s: %s", field, msg))
	}
	return strings.Join(errs, ", ")
}

// Validate validates a struct and returns validation errors if any
func Validate(s interface{}) error {
	err := validate.Struct(s)
	if err == nil {
		return nil
	}

	validationErrors := make(map[string]string)
	for _, err := range err.(validator.ValidationErrors) {
		field := err.Field()
		tag := err.Tag()
		param := err.Param()

		// Generate user-friendly error messages
		var message string
		switch tag {
		case "required":
			message = fmt.Sprintf("%s is required", field)
		case "email":
			message = fmt.Sprintf("%s must be a valid email address", field)
		case "min":
			if kind := err.Kind(); kind == reflect.String {
				message = fmt.Sprintf("%s must be at least %s characters", field, param)
			} else {
				message = fmt.Sprintf("%s must be at least %s", field, param)
			}
		case "max":
			if kind := err.Kind(); kind == reflect.String {
				message = fmt.Sprintf("%s must be at most %s characters", field, param)
			} else {
				message = fmt.Sprintf("%s must be at most %s", field, param)
			}
		case "gte":
			message = fmt.Sprintf("%s must be greater than or equal to %s", field, param)
		case "lte":
			message = fmt.Sprintf("%s must be less than or equal to %s", field, param)
		case "len":
			message = fmt.Sprintf("%s must be %s characters", field, param)
		case "url":
			message = fmt.Sprintf("%s must be a valid URL", field)
		case "uuid":
			message = fmt.Sprintf("%s must be a valid UUID", field)
		case "oneof":
			message = fmt.Sprintf("%s must be one of: %s", field, param)
		default:
			message = fmt.Sprintf("%s is invalid", field)
		}

		validationErrors[field] = message
	}

	return ValidationErrors{Errors: validationErrors}
}

// ValidateRequest is a middleware that validates request bodies
func ValidateRequest(s interface{}) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Parse the request body into the struct
		if err := c.BodyParser(s); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		// Validate the struct
		if err := validate.Struct(s); err != nil {
			validationErrors := make(map[string]string)
			for _, err := range err.(validator.ValidationErrors) {
				field := err.Field()
				tag := err.Tag()
				param := err.Param()

				// Generate user-friendly error messages
				var message string
				switch tag {
				case "required":
					message = fmt.Sprintf("%s is required", field)
				case "email":
					message = fmt.Sprintf("%s must be a valid email address", field)
				case "min":
					if kind := err.Kind(); kind == reflect.String {
						message = fmt.Sprintf("%s must be at least %s characters", field, param)
					} else {
						message = fmt.Sprintf("%s must be at least %s", field, param)
					}
				case "max":
					if kind := err.Kind(); kind == reflect.String {
						message = fmt.Sprintf("%s must be at most %s characters", field, param)
					} else {
						message = fmt.Sprintf("%s must be at most %s", field, param)
					}
				case "gte":
					message = fmt.Sprintf("%s must be greater than or equal to %s", field, param)
				case "lte":
					message = fmt.Sprintf("%s must be less than or equal to %s", field, param)
				case "len":
					message = fmt.Sprintf("%s must be %s characters", field, param)
				case "url":
					message = fmt.Sprintf("%s must be a valid URL", field)
				case "uuid":
					message = fmt.Sprintf("%s must be a valid UUID", field)
				case "oneof":
					message = fmt.Sprintf("%s must be one of: %s", field, param)
				default:
					message = fmt.Sprintf("%s is invalid", field)
				}

				validationErrors[field] = message
			}

			return c.Status(fiber.StatusUnprocessableEntity).JSON(ValidationErrors{Errors: validationErrors})
		}

		return c.Next()
	}
}

// ValidateQuery validates query parameters
func ValidateQuery(s interface{}) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := c.QueryParser(s); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid query parameters")
		}

		if err := validate.Struct(s); err != nil {
			validationErrors := make(map[string]string)
			for _, err := range err.(validator.ValidationErrors) {
				field := err.Field()
				tag := err.Tag()
				param := err.Param()

				var message string
				switch tag {
				case "required":
					message = fmt.Sprintf("%s is required", field)
				case "min":
					message = fmt.Sprintf("%s must be at least %s", field, param)
				case "max":
					message = fmt.Sprintf("%s must be at most %s", field, param)
				default:
					message = fmt.Sprintf("%s is invalid", field)
				}

				validationErrors[field] = message
			}

			return c.Status(fiber.StatusUnprocessableEntity).JSON(ValidationErrors{Errors: validationErrors})
		}

		return c.Next()
	}
}

// ValidateParams validates path parameters
func ValidateParams(s interface{}) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := c.ParamsParser(s); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid path parameters")
		}

		if err := validate.Struct(s); err != nil {
			validationErrors := make(map[string]string)
			for _, err := range err.(validator.ValidationErrors) {
				field := err.Field()
				tag := err.Tag()

				var message string
				switch tag {
				case "required":
					message = fmt.Sprintf("%s is required", field)
				case "uuid":
					message = fmt.Sprintf("%s must be a valid UUID", field)
				default:
					message = fmt.Sprintf("%s is invalid", field)
				}

				validationErrors[field] = message
			}

			return c.Status(fiber.StatusUnprocessableEntity).JSON(ValidationErrors{Errors: validationErrors})
		}

		return c.Next()
	}
}
