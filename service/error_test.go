package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetStatusCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		statusCode       int
		statusCodeConfig string
		expectedCode     int
	}{
		{
			name:             "map string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"503"}`,
			expectedCode:     503,
		},
		{
			name:             "map int value",
			statusCode:       429,
			statusCodeConfig: `{"429":503}`,
			expectedCode:     503,
		},
		{
			name:             "skip invalid string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"bad-code"}`,
			expectedCode:     429,
		},
		{
			name:             "skip status code 200",
			statusCode:       200,
			statusCodeConfig: `{"200":503}`,
			expectedCode:     200,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			newAPIError := &types.NewAPIError{
				StatusCode: tc.statusCode,
			}
			ResetStatusCode(newAPIError, tc.statusCodeConfig)
			require.Equal(t, tc.expectedCode, newAPIError.StatusCode)
		})
	}
}

func TestRelayErrorHandlerTruncatesInvalidJSONBodyInLog(t *testing.T) {
	withDebugEnabled(t, false)

	body := strings.Repeat("b", common.LocalLogContentLimit+256)
	var logBuffer bytes.Buffer

	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logBuffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, "bad response status code 500", newAPIError.Error())
	require.Contains(t, logBuffer.String(), "[truncated")
	require.Contains(t, logBuffer.String(), fmt.Sprintf("original_length=%d", len(body)))
	require.NotContains(t, logBuffer.String(), strings.Repeat("b", common.LocalLogContentLimit+1))
}

func TestRelayErrorHandlerKeepsStructuredErrorMessage(t *testing.T) {
	message := strings.Repeat("c", common.LocalLogContentLimit+256)
	body := `{"message":"` + message + `"}`
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, message, newAPIError.Error())
}

func TestRelayErrorHandlerKeepsOpenAIErrorMessage(t *testing.T) {
	message := strings.Repeat("d", common.LocalLogContentLimit+256)
	body := `{"error":{"message":"` + message + `","type":"server_error","code":"server_error"}}`
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, message, newAPIError.Error())
}

func TestRelayErrorHandlerKeepsInvalidJSONBodyInDebugLog(t *testing.T) {
	withDebugEnabled(t, true)

	body := strings.Repeat("e", common.LocalLogContentLimit+256)
	var logBuffer bytes.Buffer

	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logBuffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.NotContains(t, logBuffer.String(), "[truncated")
	require.Contains(t, logBuffer.String(), body)
}

func TestWriteGhostVertexErrorSkipsNonGhostRelay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	apiErr := types.NewOpenAIError(
		fmt.Errorf("Gemini API key invalid: invalid_api_key"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusUnauthorized,
	)

	assert.False(t, WriteGhostVertexError(c, apiErr))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Empty(t, recorder.Body.String())
}

func TestWriteGhostVertexErrorMasksUpstreamProviderDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(GhostUpstreamChannelMetaKey, true)

	apiErr := types.NewOpenAIError(
		fmt.Errorf("Gemini API key invalid: invalid_request_error invalid_api_key x-goog-api-key"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusUnauthorized,
	)

	require.True(t, WriteGhostVertexError(c, apiErr))
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)

	var body struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, http.StatusUnauthorized, body.Error.Code)
	assert.Equal(t, string(types.ErrorTypeUpstreamError), body.Error.Type)
	assert.Equal(t, "Request had invalid authentication credentials.", body.Error.Message)

	responseText := recorder.Body.String()
	assert.NotContains(t, responseText, "Gemini")
	assert.NotContains(t, responseText, "invalid_request_error")
	assert.NotContains(t, responseText, "invalid_api_key")
	assert.NotContains(t, responseText, "x-goog-api-key")
	assert.NotContains(t, responseText, "aiplatform.googleapis.com")
}

func TestMaskGhostVertexAPIErrorMapsUnknownCustomErrorToWrappedInternal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(GhostUpstreamChannelMetaKey, true)

	apiErr := types.NewOpenAIError(
		fmt.Errorf("custom upstream exploded: Claude style_error"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)

	statusCode, vertexError := BuildGhostVertexError(apiErr)
	assert.Equal(t, http.StatusInternalServerError, statusCode)
	assert.Equal(t, http.StatusInternalServerError, vertexError.Error.Code)
	assert.Equal(t, "INTERNAL", vertexError.Error.Status)
	assert.Equal(t, "Internal error encountered.", vertexError.Error.Message)
	require.Len(t, vertexError.Error.Details, 1)
	assert.Equal(t, "type.googleapis.com/google.rpc.ErrorInfo", vertexError.Error.Details[0].Type)
	assert.Equal(t, "INTERNAL_ERROR", vertexError.Error.Details[0].Reason)
	assert.Equal(t, "aiplatform.googleapis.com", vertexError.Error.Details[0].Domain)
	vertexBody, err := common.Marshal(vertexError)
	require.NoError(t, err)
	assert.NotContains(t, string(vertexBody), "Claude")

	masked := MaskGhostVertexAPIError(c, apiErr)
	openAIError := masked.ToOpenAIError()

	assert.Equal(t, http.StatusInternalServerError, masked.StatusCode)
	assert.Equal(t, string(types.ErrorTypeOpenAIError), string(masked.GetErrorType()))
	assert.Equal(t, types.ErrorCode("500"), masked.GetErrorCode())
	assert.EqualValues(t, http.StatusInternalServerError, openAIError.Code)
	assert.Equal(t, string(types.ErrorTypeUpstreamError), openAIError.Type)
	assert.Equal(t, "Internal error encountered.", openAIError.Message)
	assert.NotContains(t, openAIError.Message, "Claude")
}

func TestWriteGhostVertexErrorMasksSelectedGhostBeforeUpstreamMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(GhostChannelSelectedKey, true)

	apiErr := types.NewErrorWithStatusCode(
		fmt.Errorf("failed to get ghost upstream channel #9: no rows"),
		types.ErrorCodeGetChannelFailed,
		http.StatusInternalServerError,
	)

	require.True(t, WriteGhostVertexError(c, apiErr))
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "ghost upstream")
	assert.NotContains(t, recorder.Body.String(), "#9")
	assert.NotContains(t, recorder.Body.String(), "no rows")
	assert.Contains(t, recorder.Body.String(), "upstream_error")
	assert.Contains(t, recorder.Body.String(), "Internal error encountered.")
	assert.NotContains(t, recorder.Body.String(), "aiplatform.googleapis.com")
}

func TestMaskGhostVertexAPIErrorMasksLogsAndLastError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(GhostUpstreamChannelMetaKey, true)

	apiErr := types.NewOpenAIError(
		fmt.Errorf("custom upstream exploded: Gemini invalid_api_key x-goog-api-key"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusUnauthorized,
	)

	masked := MaskGhostVertexAPIError(c, apiErr)

	require.NotNil(t, masked)
	assert.Equal(t, http.StatusUnauthorized, masked.StatusCode)
	assert.Equal(t, types.ErrorCode("401"), masked.GetErrorCode())
	assert.Equal(t, types.ErrorTypeOpenAIError, masked.GetErrorType())
	assert.Equal(t, "Request had invalid authentication credentials.", masked.Error())
	assert.Equal(t, string(types.ErrorTypeUpstreamError), masked.ToOpenAIError().Type)
	assert.EqualValues(t, http.StatusUnauthorized, masked.ToOpenAIError().Code)
	assert.NotContains(t, masked.MaskSensitiveErrorWithStatusCode(), "Gemini")
	assert.NotContains(t, masked.MaskSensitiveErrorWithStatusCode(), "invalid_api_key")
	assert.NotContains(t, masked.MaskSensitiveErrorWithStatusCode(), "x-goog-api-key")
}

func TestGenerateTextOtherInfoMasksGhostStreamStatusErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(GhostUpstreamChannelMetaKey, true)

	streamStatus := relaycommon.NewStreamStatus()
	streamStatus.SetEndReason(relaycommon.StreamEndReasonHandlerStop, fmt.Errorf("Gemini stream invalid_api_key"))
	streamStatus.RecordError("x-goog-api-key leaked from upstream")
	now := time.Now()
	info := &relaycommon.RelayInfo{
		IsStream:          true,
		StreamStatus:      streamStatus,
		StartTime:         now,
		FirstResponseTime: now,
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}

	other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 0, 0, 0)
	streamInfo, ok := other["stream_status"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "Request had invalid authentication credentials.", streamInfo["end_error"])
	messages, ok := streamInfo["errors"].([]string)
	require.True(t, ok)
	require.Len(t, messages, 1)
	assert.NotContains(t, streamInfo["end_error"], "Gemini")
	assert.NotContains(t, streamInfo["end_error"], "invalid_api_key")
	assert.NotContains(t, messages[0], "x-goog-api-key")
	assert.NotContains(t, messages[0], "upstream")
}

func withDebugEnabled(t *testing.T, enabled bool) {
	t.Helper()

	oldDebug := common.DebugEnabled
	common.DebugEnabled = enabled
	t.Cleanup(func() {
		common.DebugEnabled = oldDebug
	})
}
