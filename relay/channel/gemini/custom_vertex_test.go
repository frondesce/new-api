package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func customGeminiVertexInfo(stream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		IsStream:        stream,
		OriginModelName: "gemini-future-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeCustom,
			ChannelBaseUrl:    "https://gateway.example.com/provider/v1/projects/project/locations/global/publishers/google/models/{model}:{action}",
			UpstreamModelName: "gemini-future-model",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				CustomProtocol: dto.CustomChannelProtocolGeminiVertex,
			},
		},
	}
}

func TestCustomGeminiVertexRequestURL(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	requestURL, err := adaptor.GetRequestURL(customGeminiVertexInfo(false))

	require.NoError(t, err)
	require.Equal(
		t,
		"https://gateway.example.com/provider/v1/projects/project/locations/global/publishers/google/models/gemini-future-model:generateContent",
		requestURL,
	)
}

func TestCustomGeminiVertexStreamRequestURL(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	requestURL, err := adaptor.GetRequestURL(customGeminiVertexInfo(true))

	require.NoError(t, err)
	require.Equal(
		t,
		"https://gateway.example.com/provider/v1/projects/project/locations/global/publishers/google/models/gemini-future-model:streamGenerateContent?alt=sse",
		requestURL,
	)
}

func TestCustomGeminiVertexStreamRequestURLWithFixedGenerateAction(t *testing.T) {
	t.Parallel()

	info := customGeminiVertexInfo(true)
	info.ChannelBaseUrl = "https://gateway.example.com/provider/v1/projects/project/locations/global/publishers/google/models/{model}:generateContent"

	adaptor := &Adaptor{}
	requestURL, err := adaptor.GetRequestURL(info)

	require.NoError(t, err)
	require.Equal(
		t,
		"https://gateway.example.com/provider/v1/projects/project/locations/global/publishers/google/models/gemini-future-model:streamGenerateContent?alt=sse",
		requestURL,
	)
}

func TestCustomGeminiVertexRequestURLEscapesModelPathSegment(t *testing.T) {
	t.Parallel()

	info := customGeminiVertexInfo(false)
	info.UpstreamModelName = "vendor/model?preview"

	adaptor := &Adaptor{}
	requestURL, err := adaptor.GetRequestURL(info)

	require.NoError(t, err)
	require.Equal(
		t,
		"https://gateway.example.com/provider/v1/projects/project/locations/global/publishers/google/models/vendor%2Fmodel%3Fpreview:generateContent",
		requestURL,
	)
}

func TestCustomGeminiVertexRequestURLRejectsActionOutsidePath(t *testing.T) {
	t.Parallel()

	info := customGeminiVertexInfo(false)
	info.ChannelBaseUrl = "https://gateway.example.com/provider/v1/models/{model}?target=:generateContent"

	adaptor := &Adaptor{}
	_, err := adaptor.GetRequestURL(info)

	require.ErrorContains(t, err, "URL path must target :generateContent")
}

func TestCustomGeminiVertexRequestURLRequiresAbsoluteURL(t *testing.T) {
	t.Parallel()

	info := customGeminiVertexInfo(false)
	info.ChannelBaseUrl = "/provider/v1/models/{model}:generateContent"

	adaptor := &Adaptor{}
	_, err := adaptor.GetRequestURL(info)

	require.ErrorContains(t, err, "must be an absolute URL")
}

func TestNativeGoogleSearchToolConvertsToGeminiGoogleSearch(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	var request dto.GeneralOpenAIRequest
	err := appcommon.Unmarshal([]byte(`{
		"model": "gemini-future-model",
		"messages": [
			{"role": "user", "content": "Search for current Vertex AI news"}
		],
		"tools": [
			{"googleSearch": {}}
		],
		"tool_choice": "auto"
	}`), &request)
	require.NoError(t, err)
	require.NotNil(t, request.Tools[0].GoogleSearch)

	converted, err := CovertOpenAI2Gemini(c, request, customGeminiVertexInfo(false))

	require.NoError(t, err)
	tools := converted.GetTools()
	require.Len(t, tools, 1)
	require.NotNil(t, tools[0].GoogleSearch)
	require.Nil(t, tools[0].FunctionDeclarations)
	require.Nil(t, converted.ToolConfig)

	jsonData, err := appcommon.Marshal(converted)
	require.NoError(t, err)
	require.JSONEq(t, `[{"googleSearch":{}}]`, string(converted.Tools))
	require.Contains(t, string(jsonData), `"googleSearch":{}`)
}

func TestGeminiChatHandlerKeepsGroundedAnswerContent(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := customGeminiVertexInfo(false)
	info.RelayFormat = types.RelayFormatOpenAI

	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader(`{
			"candidates": [{
				"content": {
					"role": "model",
					"parts": [{"text": "Grounded answer from current search results."}]
				},
				"finishReason": "STOP",
				"index": 0,
				"groundingMetadata": {
					"webSearchQueries": ["current Vertex AI news"],
					"groundingChunks": [{
						"web": {
							"uri": "https://example.com/source",
							"title": "Example source"
						}
					}]
				}
			}],
			"usageMetadata": {
				"promptTokenCount": 10,
				"candidatesTokenCount": 8,
				"totalTokenCount": 18
			}
		}`)),
	}

	usage, newAPIError := GeminiChatHandler(c, info, resp)

	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	var converted dto.OpenAITextResponse
	require.NoError(t, appcommon.Unmarshal(recorder.Body.Bytes(), &converted))
	require.Len(t, converted.Choices, 1)
	require.Equal(t, "Grounded answer from current search results.", converted.Choices[0].Message.StringContent())
}
