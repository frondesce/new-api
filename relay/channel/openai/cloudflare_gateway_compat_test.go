package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestShouldNormalizeCloudflareGatewayToolCallIndexes(t *testing.T) {
	t.Parallel()

	require.True(t, shouldNormalizeCloudflareGatewayToolCallIndexes(&relaycommon.RelayInfo{
		IsStream:  true,
		RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://gateway.ai.cloudflare.com/v1/account/gateway/compat/chat/completions",
		},
	}))

	require.False(t, shouldNormalizeCloudflareGatewayToolCallIndexes(&relaycommon.RelayInfo{
		IsStream:  false,
		RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://gateway.ai.cloudflare.com/v1/account/gateway/compat/chat/completions",
		},
	}))

	require.False(t, shouldNormalizeCloudflareGatewayToolCallIndexes(&relaycommon.RelayInfo{
		IsStream:  true,
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://gateway.ai.cloudflare.com/v1/account/gateway/compat/chat/completions",
		},
	}))

	require.False(t, shouldNormalizeCloudflareGatewayToolCallIndexes(&relaycommon.RelayInfo{
		IsStream:  true,
		RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://api.openrouter.ai/v1",
		},
	}))
}

func TestShouldInjectCloudflareGatewayGeminiThoughtSignature(t *testing.T) {
	t.Parallel()

	originEnabled := model_setting.GetGeminiSettings().FunctionCallThoughtSignatureEnabled
	t.Cleanup(func() {
		model_setting.GetGeminiSettings().FunctionCallThoughtSignatureEnabled = originEnabled
	})
	model_setting.GetGeminiSettings().FunctionCallThoughtSignatureEnabled = true

	require.True(t, shouldInjectCloudflareGatewayGeminiThoughtSignature(&relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://gateway.ai.cloudflare.com/v1/account/gateway/compat/chat/completions",
			UpstreamModelName: "google-ai-studio/gemini-3-flash-preview",
		},
	}, &dto.GeneralOpenAIRequest{
		Model: "google-ai-studio/gemini-3-flash-preview",
	}))

	require.False(t, shouldInjectCloudflareGatewayGeminiThoughtSignature(&relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://gateway.ai.cloudflare.com/v1/account/gateway/compat/chat/completions",
		},
	}, &dto.GeneralOpenAIRequest{
		Model: "google-ai-studio/gemini-3-flash-preview",
	}))

	require.False(t, shouldInjectCloudflareGatewayGeminiThoughtSignature(&relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://api.openrouter.ai/v1",
		},
	}, &dto.GeneralOpenAIRequest{
		Model: "google-ai-studio/gemini-3-flash-preview",
	}))
}

func TestNormalizeCloudflareGatewayToolCallIndexes_AssignsMissingIndex(t *testing.T) {
	t.Parallel()

	state := newToolCallIndexState()
	data := `{"choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"search_web","arguments":"{\"query\":\"OceanStor\"}"}}]}}]}`

	normalized, err := normalizeCloudflareGatewayToolCallIndexes(data, state)
	require.NoError(t, err)
	require.EqualValues(t, 0, gjson.Get(normalized, "choices.0.delta.tool_calls.0.index").Int())
}

func TestNormalizeCloudflareGatewayToolCallIndexes_ReusesIndexAcrossChunks(t *testing.T) {
	t.Parallel()

	state := newToolCallIndexState()
	first := `{"choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"search_web","arguments":""}},{"id":"call_2","type":"function","function":{"name":"fetch_url","arguments":""}}]}}]}`
	second := `{"choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_2","type":"function","function":{"name":"","arguments":"{\"url\":\"https://example.com\"}"}}]}}]}`

	normalizedFirst, err := normalizeCloudflareGatewayToolCallIndexes(first, state)
	require.NoError(t, err)

	normalizedSecond, err := normalizeCloudflareGatewayToolCallIndexes(second, state)
	require.NoError(t, err)

	require.EqualValues(t, 0, gjson.Get(normalizedFirst, "choices.0.delta.tool_calls.0.index").Int())
	require.EqualValues(t, 1, gjson.Get(normalizedFirst, "choices.0.delta.tool_calls.1.index").Int())
	require.EqualValues(t, 1, gjson.Get(normalizedSecond, "choices.0.delta.tool_calls.0.index").Int())
}

func TestNormalizeCloudflareGatewayToolCallIndexes_LeavesExistingIndexUntouched(t *testing.T) {
	t.Parallel()

	state := newToolCallIndexState()
	data := `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":3,"id":"call_1","type":"function","function":{"name":"search_web","arguments":"{}"}}]}}]}`

	normalized, err := normalizeCloudflareGatewayToolCallIndexes(data, state)
	require.NoError(t, err)
	require.JSONEq(t, data, normalized)
}

func TestInjectCloudflareGatewayGeminiThoughtSignature_AddsBypassSignature(t *testing.T) {
	t.Parallel()

	request := &dto.GeneralOpenAIRequest{
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "今天是几号来着？",
			},
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: []byte(`[
					{
						"id":"z2lk5c5p",
						"type":"function",
						"function":{"name":"get_current_timestamp","arguments":"{}"}
					}
				]`),
			},
		},
	}

	err := injectCloudflareGatewayGeminiThoughtSignature(request)
	require.NoError(t, err)
	require.Equal(
		t,
		cloudflareGatewayGeminiThoughtSignatureBypassValue,
		gjson.GetBytes(request.Messages[1].ToolCalls, "0.extra_content.google.thought_signature").String(),
	)
}

func TestInjectCloudflareGatewayGeminiThoughtSignature_LeavesExistingSignatureUntouched(t *testing.T) {
	t.Parallel()

	request := &dto.GeneralOpenAIRequest{
		Messages: []dto.Message{
			{
				Role:    "assistant",
				Content: "",
				ToolCalls: []byte(`[
					{
						"id":"z2lk5c5p",
						"type":"function",
						"extra_content":{"google":{"thought_signature":"original_signature"}},
						"function":{"name":"get_current_timestamp","arguments":"{}"}
					}
				]`),
			},
		},
	}

	err := injectCloudflareGatewayGeminiThoughtSignature(request)
	require.NoError(t, err)
	require.Equal(
		t,
		"original_signature",
		gjson.GetBytes(request.Messages[0].ToolCalls, "0.extra_content.google.thought_signature").String(),
	)
}
