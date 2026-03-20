package openai

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURL_UsesChatCompletionsForMiMoAudioSpeech(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://api.xiaomimimo.com",
			UpstreamModelName: "mimo-v2-tts",
		},
	}

	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://api.xiaomimimo.com/v1/chat/completions", url)
}

func TestSetupRequestHeader_UsesAPIKeyForMiMoTTS(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("Accept", "audio/mpeg")

	adaptor := &Adaptor{}
	header := http.Header{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "mimo-key",
			UpstreamModelName: "mimo-v2-tts",
		},
	}

	err := adaptor.SetupRequestHeader(ctx, &header, info)
	require.NoError(t, err)
	require.Equal(t, "mimo-key", header.Get("api-key"))
	require.Equal(t, "application/json", header.Get("Accept"))
	require.Equal(t, "application/json", header.Get("Content-Type"))
	require.Empty(t, header.Get("Authorization"))
}

func TestConvertAudioRequest_ConvertsMiMoTTSIntoChatCompletionsBody(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{}`))

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "mimo-v2-tts",
		},
	}

	bodyReader, err := adaptor.ConvertAudioRequest(ctx, info, dto.AudioRequest{
		Model:          "mimo-v2-tts",
		Input:          "hello world",
		Voice:          "alloy",
		Instructions:   "Speak warmly.",
		ResponseFormat: "wav",
	})
	require.NoError(t, err)

	bodyBytes := make([]byte, 0)
	bodyBytes, err = io.ReadAll(bodyReader)
	require.NoError(t, err)

	var payload map[string]any
	err = common.Unmarshal(bodyBytes, &payload)
	require.NoError(t, err)
	require.Equal(t, "mimo-v2-tts", payload["model"])

	messages, ok := payload["messages"].([]any)
	require.True(t, ok)
	require.Len(t, messages, 2)

	userMessage, ok := messages[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "user", userMessage["role"])
	require.Equal(t, "Please synthesize the following assistant message into speech. Follow these speaking instructions: Speak warmly.", userMessage["content"])

	assistantMessage, ok := messages[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "assistant", assistantMessage["role"])
	require.Equal(t, "hello world", assistantMessage["content"])

	audio, ok := payload["audio"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "wav", audio["format"])
	require.Equal(t, "mimo_default", audio["voice"])
}

func TestConvertAudioRequest_MapsSpeedIntoMiMoStyleTag(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{}`))

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "mimo-v2-tts",
		},
	}

	speed := 0.95
	bodyReader, err := adaptor.ConvertAudioRequest(ctx, info, dto.AudioRequest{
		Model: "mimo-v2-tts",
		Input: "hello world",
		Speed: &speed,
	})
	require.NoError(t, err)

	bodyBytes, err := io.ReadAll(bodyReader)
	require.NoError(t, err)

	var payload map[string]any
	err = common.Unmarshal(bodyBytes, &payload)
	require.NoError(t, err)

	messages, ok := payload["messages"].([]any)
	require.True(t, ok)
	require.Len(t, messages, 2)

	userMessage, ok := messages[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "user", userMessage["role"])
	require.Equal(t, miMoTTSSynthesisPrompt, userMessage["content"])

	assistantMessage, ok := messages[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "assistant", assistantMessage["role"])
	require.Equal(t, "<style>Slow down</style>hello world", assistantMessage["content"])
}

func TestApplyMiMoSpeedStyle_MergesIntoExistingStyleTag(t *testing.T) {
	t.Parallel()

	speed := 1.2
	output := applyMiMoSpeedStyle("<style>Happy</style>Hello", &speed)
	require.Equal(t, "<style>Happy Speed up</style>Hello", output)
}

func TestApplyMiMoSpeedStyle_LeavesExactOneUnchanged(t *testing.T) {
	t.Parallel()

	speed := 1.0
	output := applyMiMoSpeedStyle("Hello", &speed)
	require.Equal(t, "Hello", output)
}

func TestMiMoTTSHandler_DecodesChatAudioPayload(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{}`))

	audioBytes := []byte("fake-audio")
	responseBody := `{
		"choices": [
			{
				"message": {
					"audio": {
						"data": "` + base64.StdEncoding.EncodeToString(audioBytes) + `",
						"format": "wav"
					}
				}
			}
		],
		"usage": {
			"prompt_tokens": 12,
			"completion_tokens": 34,
			"total_tokens": 46
		}
	}`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(responseBody)),
	}
	info := &relaycommon.RelayInfo{
		Request: &dto.AudioRequest{
			ResponseFormat: "wav",
		},
	}

	usage, err := MiMoTTSHandler(ctx, resp, info)
	require.Nil(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "audio/wav", recorder.Header().Get("Content-Type"))
	require.Equal(t, audioBytes, recorder.Body.Bytes())
	require.Equal(t, 12, usage.PromptTokens)
	require.Equal(t, 34, usage.CompletionTokens)
	require.Equal(t, 46, usage.TotalTokens)
	require.Equal(t, 34, usage.CompletionTokenDetails.AudioTokens)
}
