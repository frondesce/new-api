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

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
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

func TestRelayErrorHandlerPreservesRateLimitErrorsAndConfiguredPolicy(t *testing.T) {
	previousEnabled := common.AutomaticDisableChannelEnabled
	previousRetryRanges := operation_setting.AutomaticRetryStatusCodeRanges
	previousDisableRanges := operation_setting.AutomaticDisableStatusCodeRanges
	previousKeywords := operation_setting.AutomaticDisableKeywords
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = previousEnabled
		operation_setting.AutomaticRetryStatusCodeRanges = previousRetryRanges
		operation_setting.AutomaticDisableStatusCodeRanges = previousDisableRanges
		operation_setting.AutomaticDisableKeywords = previousKeywords
	})
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 429, End: 429}}
	operation_setting.AutomaticDisableStatusCodeRanges = nil
	operation_setting.AutomaticDisableKeywords = []string{"credit balance is too low", "余额不足"}

	for _, tc := range []struct {
		name        string
		body        string
		message     string
		errorType   string
		code        string
		wantDisable bool
	}{
		{
			name:    "Chinese balance error uses configured keyword",
			body:    `{"error":{"message":"余额不足","type":"upstream_error","code":"upstream_error"}}`,
			message: "余额不足", errorType: "upstream_error", code: "upstream_error", wantDisable: true,
		},
		{
			name:    "per minute quota remains retryable",
			body:    `{"error":{"message":"quota exceeded for requests per minute","type":"rate_limit_error","code":"rate_limit_exceeded"}}`,
			message: "quota exceeded for requests per minute", errorType: "rate_limit_error", code: "rate_limit_exceeded",
		},
		{
			name:    "concurrency error keeps provider classification",
			body:    `{"error":{"message":"当前并发数已达到上限","type":"upstream_error","code":"concurrency_limit"}}`,
			message: "当前并发数已达到上限", errorType: "upstream_error", code: "concurrency_limit",
		},
		{
			name:    "structured quota error permits configured channel disable",
			body:    `{"error":{"message":"credit balance is too low","type":"insufficient_quota","code":"insufficient_quota"}}`,
			message: "credit balance is too low", errorType: "insufficient_quota", code: "insufficient_quota", wantDisable: true,
		},
		{
			name:    "metadata is appended once",
			body:    `{"error":{"message":"busy","type":"rate_limit_error","code":"rate_limit_exceeded","metadata":{"request_id":"metadata-1"}}}`,
			message: `busy ({"request_id":"metadata-1"})`, errorType: "rate_limit_error", code: "rate_limit_exceeded",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(strings.NewReader(tc.body)),
			}
			apiErr := RelayErrorHandler(context.Background(), resp, false)
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
			assert.Equal(t, tc.message, apiErr.Error())
			assert.Equal(t, types.ErrorCode(tc.code), apiErr.GetErrorCode())
			assert.False(t, types.IsSkipRetryError(apiErr))
			openAIError := apiErr.ToOpenAIError()
			assert.Equal(t, tc.errorType, openAIError.Type)
			assert.Equal(t, tc.code, openAIError.Code)

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			assert.Equal(t, "retry", DecideRelayRetry(c, apiErr, 1).Action)
			assert.Equal(t, "stop", DecideRelayRetry(c, apiErr, 0).Action)
			assert.Equal(t, tc.wantDisable, ShouldDisableChannel(apiErr))

			operation_setting.AutomaticRetryStatusCodeRanges = nil
			assert.Equal(t, "stop", DecideRelayRetry(c, apiErr, 1).Action)
			operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 429, End: 429}}
		})
	}
}

func TestRelayErrorHandlerPreservesNonRateLimitErrors(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Body: io.NopCloser(strings.NewReader(
			`{"error":{"message":"配额不足","type":"upstream_error","code":"upstream_error"}}`,
		)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, http.StatusForbidden, newAPIError.StatusCode)
	require.Equal(t, "配额不足", newAPIError.Error())
	require.False(t, types.IsSkipRetryError(newAPIError))
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

func withDebugEnabled(t *testing.T, enabled bool) {
	t.Helper()

	oldDebug := common.DebugEnabled
	common.DebugEnabled = enabled
	t.Cleanup(func() {
		common.DebugEnabled = oldDebug
	})
}
