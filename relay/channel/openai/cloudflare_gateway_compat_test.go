package openai

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
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
