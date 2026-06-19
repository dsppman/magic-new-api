package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	GhostChannelSelectedKey                  = "__ghost_channel_selected"
	GhostUpstreamChannelMetaKey              = "__ghost_upstream_channel_meta"
	GhostUpstreamChannelModelMappingKey      = "__ghost_upstream_channel_model_mapping"
	GhostUpstreamChannelStatusCodeMappingKey = "__ghost_upstream_channel_status_code_mapping"
	GhostUpstreamChannelOtherKey             = "__ghost_upstream_channel_other"
)

type vertexAIErrorTemplate struct {
	code    int
	status  string
	message string
	reason  string
}

type vertexAIErrorResponse struct {
	Error vertexAIError `json:"error"`
}

type vertexAIError struct {
	Code    int                   `json:"code"`
	Message string                `json:"message"`
	Status  string                `json:"status"`
	Details []vertexAIErrorDetail `json:"details,omitempty"`
}

type vertexAIErrorDetail struct {
	Type     string            `json:"@type"`
	Reason   string            `json:"reason,omitempty"`
	Domain   string            `json:"domain,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func IsGhostChannelRelay(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if _, ok := c.Get(GhostChannelSelectedKey); ok {
		return true
	}
	_, ok := c.Get(GhostUpstreamChannelMetaKey)
	return ok
}

func WriteGhostVertexError(c *gin.Context, apiErr *types.NewAPIError) bool {
	if !IsGhostChannelRelay(c) || apiErr == nil {
		return false
	}
	maskedErr := MaskGhostVertexAPIError(c, apiErr)
	c.JSON(maskedErr.StatusCode, gin.H{
		"error": maskedErr.ToOpenAIError(),
	})
	return true
}

func MaskGhostVertexAPIError(c *gin.Context, apiErr *types.NewAPIError) *types.NewAPIError {
	if !IsGhostChannelRelay(c) || apiErr == nil {
		return apiErr
	}

	options := make([]types.NewAPIErrorOptions, 0, 2)
	if types.IsSkipRetryError(apiErr) {
		options = append(options, types.ErrOptionWithSkipRetry())
	}
	if !types.IsRecordErrorLog(apiErr) {
		options = append(options, types.ErrOptionWithNoRecordErrorLog())
	}

	statusCode, vertexErr := BuildGhostVertexError(apiErr)
	body, err := common.Marshal(vertexErr)
	if err != nil {
		tmpl := ghostVertexErrorTemplateFor(apiErr)
		return buildGhostFallbackOpenAIError(tmpl, options...)
	}

	maskedErr := RelayErrorHandler(ghostRequestContext(c), &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, false)
	return applyGhostErrorOptions(maskedErr, options...)
}

func MaskGhostErrorMessage(c *gin.Context, message string, statusCode int) string {
	if !IsGhostChannelRelay(c) || message == "" {
		return message
	}
	apiErr := types.NewOpenAIError(errors.New(message), types.ErrorCodeBadResponseStatusCode, statusCode)
	return MaskGhostVertexAPIError(c, apiErr).Error()
}

func MaskGhostRejectReason(c *gin.Context, reason string) string {
	if !IsGhostChannelRelay(c) || reason == "" {
		return reason
	}
	text := strings.ToLower(reason)
	switch {
	case strings.Contains(text, "block") || strings.Contains(text, "filter") || strings.Contains(text, "refusal"):
		return "block_reason=SAFETY"
	case strings.Contains(text, "empty"):
		return "empty_candidates"
	default:
		return "request_rejected"
	}
}

func BuildGhostVertexError(apiErr *types.NewAPIError) (int, vertexAIErrorResponse) {
	tmpl := ghostVertexErrorTemplateFor(apiErr)
	detail := vertexAIErrorDetail{
		Type:   "type.googleapis.com/google.rpc.ErrorInfo",
		Reason: tmpl.reason,
		Domain: "aiplatform.googleapis.com",
	}
	return tmpl.code, vertexAIErrorResponse{
		Error: vertexAIError{
			Code:    tmpl.code,
			Message: tmpl.message,
			Status:  tmpl.status,
			Details: []vertexAIErrorDetail{detail},
		},
	}
}

func buildGhostFallbackOpenAIError(tmpl vertexAIErrorTemplate, options ...types.NewAPIErrorOptions) *types.NewAPIError {
	return types.WithOpenAIError(
		types.OpenAIError{
			Message: tmpl.message,
			Code:    tmpl.code,
		},
		tmpl.code,
		options...,
	)
}

func applyGhostErrorOptions(apiErr *types.NewAPIError, options ...types.NewAPIErrorOptions) *types.NewAPIError {
	if apiErr == nil || len(options) == 0 {
		return apiErr
	}
	return types.WithOpenAIError(apiErr.ToOpenAIError(), apiErr.StatusCode, options...)
}

func ghostRequestContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func ghostVertexErrorTemplateFor(apiErr *types.NewAPIError) vertexAIErrorTemplate {
	statusCode := apiErr.StatusCode
	text := strings.ToLower(string(apiErr.GetErrorCode()) + " " + apiErr.Error())

	switch {
	case strings.Contains(text, "prompt_blocked") ||
		strings.Contains(text, "safety") ||
		strings.Contains(text, "blocked") ||
		strings.Contains(text, "prohibited"):
		return vertexAIErrorTemplate{
			code:    http.StatusBadRequest,
			status:  "FAILED_PRECONDITION",
			message: "The prompt was blocked because it violates safety policies.",
			reason:  "CONTENT_FILTERED",
		}
	case statusCode == http.StatusUnauthorized ||
		strings.Contains(text, "unauthenticated") ||
		strings.Contains(text, "authentication") ||
		strings.Contains(text, "credential") ||
		strings.Contains(text, "invalid api key") ||
		strings.Contains(text, "invalid_api_key") ||
		strings.Contains(text, "api key"):
		return vertexAIErrorTemplate{
			code:    http.StatusUnauthorized,
			status:  "UNAUTHENTICATED",
			message: "Request had invalid authentication credentials.",
			reason:  "AUTHENTICATION_ERROR",
		}
	case statusCode == http.StatusForbidden ||
		strings.Contains(text, "permission") ||
		strings.Contains(text, "denied") ||
		strings.Contains(text, "suspended") ||
		strings.Contains(text, "billing") ||
		strings.Contains(text, "disabled"):
		return vertexAIErrorTemplate{
			code:    http.StatusForbidden,
			status:  "PERMISSION_DENIED",
			message: "The caller does not have permission.",
			reason:  "IAM_PERMISSION_DENIED",
		}
	case statusCode == http.StatusNotFound ||
		strings.Contains(text, "not found") ||
		strings.Contains(text, "model_not_found"):
		return vertexAIErrorTemplate{
			code:    http.StatusNotFound,
			status:  "NOT_FOUND",
			message: "Requested entity was not found.",
			reason:  "RESOURCE_NOT_FOUND",
		}
	case statusCode == http.StatusTooManyRequests ||
		strings.Contains(text, "quota") ||
		strings.Contains(text, "rate limit") ||
		strings.Contains(text, "rate_limit") ||
		strings.Contains(text, "resource exhausted") ||
		strings.Contains(text, "too many requests"):
		return vertexAIErrorTemplate{
			code:    http.StatusTooManyRequests,
			status:  "RESOURCE_EXHAUSTED",
			message: "Quota exceeded for quota metric 'Generate Content requests' and limit 'Generate content requests per minute'.",
			reason:  "RATE_LIMIT_EXCEEDED",
		}
	case statusCode == 499 ||
		strings.Contains(text, "cancelled") ||
		strings.Contains(text, "canceled"):
		return vertexAIErrorTemplate{
			code:    499,
			status:  "CANCELLED",
			message: "Request cancelled by the client.",
			reason:  "CLIENT_CANCELLED",
		}
	case statusCode == http.StatusGatewayTimeout ||
		statusCode == http.StatusRequestTimeout ||
		strings.Contains(text, "deadline") ||
		strings.Contains(text, "timeout") ||
		strings.Contains(text, "timed out"):
		return vertexAIErrorTemplate{
			code:    http.StatusGatewayTimeout,
			status:  "DEADLINE_EXCEEDED",
			message: "Deadline expired before operation could complete.",
			reason:  "DEADLINE_EXCEEDED",
		}
	case statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusBadGateway ||
		strings.Contains(text, "unavailable") ||
		strings.Contains(text, "overload") ||
		strings.Contains(text, "overloaded") ||
		strings.Contains(text, "try again"):
		return vertexAIErrorTemplate{
			code:    http.StatusServiceUnavailable,
			status:  "UNAVAILABLE",
			message: "The service is currently unavailable.",
			reason:  "SERVICE_UNAVAILABLE",
		}
	case statusCode == http.StatusBadRequest ||
		strings.Contains(text, "invalid") ||
		strings.Contains(text, "unsupported") ||
		strings.Contains(text, "bad request") ||
		strings.Contains(text, "token limit"):
		return vertexAIErrorTemplate{
			code:    http.StatusBadRequest,
			status:  "INVALID_ARGUMENT",
			message: "Request contains an invalid argument.",
			reason:  "INVALID_ARGUMENT",
		}
	case statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError:
		return vertexAIErrorTemplate{
			code:    http.StatusBadRequest,
			status:  "INVALID_ARGUMENT",
			message: "Request contains an invalid argument.",
			reason:  "INVALID_ARGUMENT",
		}
	default:
		return vertexAIErrorTemplate{
			code:    http.StatusInternalServerError,
			status:  "INTERNAL",
			message: "Internal error encountered.",
			reason:  "INTERNAL_ERROR",
		}
	}
}
