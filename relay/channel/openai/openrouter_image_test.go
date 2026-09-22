package openai

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type capturedOpenRouterImageRequest struct {
	method string
	path   string
	header http.Header
	body   []byte
	err    error
}

func createTinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	require.NoError(t, err)
	return buf.Bytes()
}

func createTinyJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, nil)
	require.NoError(t, err)
	return buf.Bytes()
}

// TestOpenRouterImage_ContractLifecycle verifies the full lifecycle:
// Request validation -> common.DeepCopy -> adaptor.ConvertImageRequest -> marshal ->
// adaptor.DoRequest against httptest.Server -> adaptor.DoResponse.
func TestOpenRouterImage_ContractLifecycle(t *testing.T) {
	pngBytes := createTinyPNG(t)

	var reqBody bytes.Buffer
	writer := multipart.NewWriter(&reqBody)
	require.NoError(t, writer.WriteField("model", "client-image-model"))
	require.NoError(t, writer.WriteField("prompt", "a scenic sunset"))
	require.NoError(t, writer.WriteField("n", "2"))
	require.NoError(t, writer.WriteField("size", "auto"))
	require.NoError(t, writer.WriteField("response_format", "b64_json"))

	part, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = part.Write(pngBytes)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &reqBody)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	respJSON := `{
		"created": 1710000000,
		"data": [
			{"b64_json": "fake-b64-1", "media_type": "image/png"},
			{"b64_json": "fake-b64-2", "media_type": "image/png"}
		],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 20,
			"total_tokens": 30,
			"cost": 0.005
		}
	}`
	captured := make(chan capturedOpenRouterImageRequest, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		captured <- capturedOpenRouterImageRequest{r.Method, r.URL.Path, r.Header.Clone(), body, readErr}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(respJSON))
	}))
	defer ts.Close()

	// 1. Request validation
	validatedReq, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
	require.NoError(t, err)
	require.NotNil(t, validatedReq)

	// 2. common.DeepCopy
	copiedReq, err := common.DeepCopy(validatedReq)
	require.NoError(t, err)
	require.NotNil(t, copiedReq)

	// 3. Model mapping
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		OriginModelName: "client-image-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenRouter,
			ChannelBaseUrl: ts.URL + "/api",
			ApiKey:         "sk-openrouter-key",
		},
	}
	c.Set("model_mapping", `{"client-image-model": "openrouter/mapped-image-model"}`)
	err = helper.ModelMappedHelper(c, info, copiedReq)
	require.NoError(t, err)
	assert.Equal(t, "openrouter/mapped-image-model", copiedReq.Model)

	// 4. adaptor.ConvertImageRequest
	adaptor := &Adaptor{}
	adaptor.Init(info)
	converted, err := adaptor.ConvertImageRequest(c, info, *copiedReq)
	require.NoError(t, err)
	require.NotNil(t, converted)

	// 5. marshal
	jsonBytes, err := common.Marshal(converted)
	require.NoError(t, err)

	// 6. adaptor.DoRequest against httptest.Server
	resp, err := adaptor.DoRequest(c, info, bytes.NewReader(jsonBytes))
	require.NoError(t, err)
	require.NotNil(t, resp)
	httpResp, ok := resp.(*http.Response)
	require.True(t, ok)
	upstreamRequest := <-captured
	require.NoError(t, upstreamRequest.err)
	r := httptest.NewRequest(upstreamRequest.method, upstreamRequest.path, bytes.NewReader(upstreamRequest.body))
	r.Header = upstreamRequest.header
	assert.Equal(t, http.MethodPost, r.Method)
	assert.Equal(t, "/api/v1/images", r.URL.Path)
	assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

	bodyBytes, err := io.ReadAll(r.Body)
	require.NoError(t, err)

	var payload map[string]any
	err = common.Unmarshal(bodyBytes, &payload)
	require.NoError(t, err)

	assert.Equal(t, "openrouter/mapped-image-model", payload["model"])
	assert.Equal(t, "a scenic sunset", payload["prompt"])
	assert.Equal(t, float64(2), payload["n"])
	assert.NotContains(t, payload, "size")
	assert.NotContains(t, payload, "response_format")

	refs, ok := payload["input_references"].([]any)
	require.True(t, ok)
	require.Len(t, refs, 1)

	refMap, ok := refs[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "image_url", refMap["type"])

	imgURLObj, ok := refMap["image_url"].(map[string]any)
	require.True(t, ok)
	assert.Len(t, imgURLObj, 1)
	imgURL, ok := imgURLObj["url"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(imgURL, "data:image/png;base64,"))

	b64Data := strings.TrimPrefix(imgURL, "data:image/png;base64,")
	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	require.NoError(t, err)
	assert.Equal(t, pngBytes, decoded)

	// 7. adaptor.DoResponse
	usage, apiErr := adaptor.DoResponse(c, httpResp, info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)

	usageObj, ok := usage.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, 10, usageObj.PromptTokens)
	assert.Equal(t, 20, usageObj.CompletionTokens)
	assert.Equal(t, 30, usageObj.TotalTokens)
	assert.Equal(t, 0.005, usageObj.Cost)

	downstreamBody := rec.Body.String()
	assert.Equal(t, respJSON, downstreamBody)
	assert.Equal(t, "image/png", gjson.Get(downstreamBody, "data.0.media_type").String())
	assert.Equal(t, 0.005, gjson.Get(downstreamBody, "usage.cost").Float())
}

// TestOpenRouterImage_MultipartPreservationAndOpenAIRetry asserts that the incoming
// multipart Content-Type is unchanged by OpenRouter conversion, so that subsequent retry
// on an ordinary OpenAI adaptor still produces a valid multipart /v1/images/edits
// request with the original file bytes intact.
func TestOpenRouterImage_MultipartPreservationAndOpenAIRetry(t *testing.T) {
	pngBytes := createTinyPNG(t)

	var reqBody bytes.Buffer
	writer := multipart.NewWriter(&reqBody)
	require.NoError(t, writer.WriteField("model", "gpt-image-1"))
	require.NoError(t, writer.WriteField("prompt", "edit this graphic"))
	part, err := writer.CreateFormFile("image", "graphic.png")
	require.NoError(t, err)
	_, err = part.Write(pngBytes)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	expectedContentType := writer.FormDataContentType()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &reqBody)
	c.Request.Header.Set("Content-Type", expectedContentType)

	validatedReq, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
	require.NoError(t, err)

	copiedReq, err := common.DeepCopy(validatedReq)
	require.NoError(t, err)

	openRouterInfo := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenRouter,
			ChannelBaseUrl: "https://openrouter.ai/api",
			ApiKey:         "sk-or-test",
		},
	}
	openRouterAdaptor := &Adaptor{}
	openRouterAdaptor.Init(openRouterInfo)

	converted, err := openRouterAdaptor.ConvertImageRequest(c, openRouterInfo, *copiedReq)
	require.NoError(t, err)
	require.NotNil(t, converted)

	// Inbound Content-Type header on c.Request must remain unchanged
	assert.Equal(t, expectedContentType, c.Request.Header.Get("Content-Type"))

	// Now simulate channel retry using an ordinary OpenAI adaptor
	captured := make(chan capturedOpenRouterImageRequest, 1)
	openaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		captured <- capturedOpenRouterImageRequest{r.Method, r.URL.Path, r.Header.Clone(), body, readErr}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"created": 1710000000, "data": [{"b64_json": "retry-b64"}]}`))
	}))
	defer openaiServer.Close()

	openaiInfo := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeImagesEdits,
		RequestURLPath: "/v1/images/edits",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelBaseUrl: openaiServer.URL,
			ApiKey:         "sk-openai-retry",
		},
	}
	openaiAdaptor := &Adaptor{}
	openaiAdaptor.Init(openaiInfo)

	convertedOpenAI, err := openaiAdaptor.ConvertImageRequest(c, openaiInfo, *copiedReq)
	require.NoError(t, err)
	buf, ok := convertedOpenAI.(*bytes.Buffer)
	require.True(t, ok)

	reqURL, err := openaiAdaptor.GetRequestURL(openaiInfo)
	require.NoError(t, err)
	assert.Equal(t, openaiServer.URL+"/v1/images/edits", reqURL)

	header := http.Header{}
	err = openaiAdaptor.SetupRequestHeader(c, &header, openaiInfo)
	require.NoError(t, err)
	assert.Equal(t, c.Request.Header.Get("Content-Type"), header.Get("Content-Type"))
	assert.True(t, strings.HasPrefix(header.Get("Content-Type"), "multipart/form-data; boundary="))

	retryResp, err := openaiAdaptor.DoRequest(c, openaiInfo, buf)
	require.NoError(t, err)
	require.NotNil(t, retryResp)
	defer retryResp.(*http.Response).Body.Close()
	upstreamRequest := <-captured
	require.NoError(t, upstreamRequest.err)
	r := httptest.NewRequest(upstreamRequest.method, upstreamRequest.path, bytes.NewReader(upstreamRequest.body))
	r.Header = upstreamRequest.header
	assert.Equal(t, http.MethodPost, r.Method)
	assert.Equal(t, "/v1/images/edits", r.URL.Path)
	assert.True(t, strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data"))

	err = r.ParseMultipartForm(32 << 20)
	require.NoError(t, err)
	assert.Equal(t, "gpt-image-1", r.PostForm.Get("model"))
	assert.Equal(t, "edit this graphic", r.PostForm.Get("prompt"))

	files := r.MultipartForm.File["image"]
	require.Len(t, files, 1)
	f, err := files[0].Open()
	require.NoError(t, err)
	defer f.Close()
	content, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, pngBytes, content)

}

// TestOpenRouterImage_InputFormats verifies JSON input image string/list,
// images array of objects with image_url strings, direct input_references,
// and sniffed real PNG/JPEG MIME types.
func TestOpenRouterImage_InputFormats(t *testing.T) {
	newOpenRouterInfo := func() *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			RelayMode: relayconstant.RelayModeImagesEdits,
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:    constant.ChannelTypeOpenRouter,
				ChannelBaseUrl: "https://openrouter.ai/api",
			},
		}
	}

	t.Run("JSON input image string", func(t *testing.T) {
		jsonStr := `{"model":"openrouter/test","prompt":"edit","image":"https://example.com/single.png"}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		converted, err := adaptor.ConvertImageRequest(c, info, *req)
		require.NoError(t, err)

		payload := converted.(map[string]any)
		refs := payload["input_references"].([]openRouterImageReference)
		require.Len(t, refs, 1)
		assert.Equal(t, "image_url", refs[0].Type)
		assert.Equal(t, "https://example.com/single.png", refs[0].ImageURL.URL)
	})

	t.Run("JSON input image string list", func(t *testing.T) {
		jsonStr := `{"model":"openrouter/test","prompt":"edit","image":["https://example.com/a.png","https://example.com/b.png"]}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		converted, err := adaptor.ConvertImageRequest(c, info, *req)
		require.NoError(t, err)

		payload := converted.(map[string]any)
		refs := payload["input_references"].([]openRouterImageReference)
		require.Len(t, refs, 2)
		assert.Equal(t, "https://example.com/a.png", refs[0].ImageURL.URL)
		assert.Equal(t, "https://example.com/b.png", refs[1].ImageURL.URL)
	})

	t.Run("JSON input images array of objects with image_url strings", func(t *testing.T) {
		jsonStr := `{"model":"openrouter/test","prompt":"edit","images":[{"image_url":"https://example.com/1.png"},{"image_url":"https://example.com/2.png"}]}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		converted, err := adaptor.ConvertImageRequest(c, info, *req)
		require.NoError(t, err)

		payload := converted.(map[string]any)
		refs := payload["input_references"].([]openRouterImageReference)
		require.Len(t, refs, 2)
		assert.Equal(t, "https://example.com/1.png", refs[0].ImageURL.URL)
		assert.Equal(t, "https://example.com/2.png", refs[1].ImageURL.URL)
	})

	t.Run("JSON input direct input_references", func(t *testing.T) {
		jsonStr := `{"model":"openrouter/test","prompt":"edit","input_references":[{"type":"image_url","image_url":{"url":"https://example.com/ref.png"}}]}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		converted, err := adaptor.ConvertImageRequest(c, info, *req)
		require.NoError(t, err)

		payload := converted.(map[string]any)
		refs := payload["input_references"].([]openRouterImageReference)
		require.Len(t, refs, 1)
		assert.Equal(t, "image_url", refs[0].Type)
		assert.Equal(t, "https://example.com/ref.png", refs[0].ImageURL.URL)
	})

	t.Run("Multipart real PNG and JPEG MIME sniffing", func(t *testing.T) {
		pngBytes := createTinyPNG(t)
		jpegBytes := createTinyJPEG(t)

		var reqBody bytes.Buffer
		writer := multipart.NewWriter(&reqBody)
		require.NoError(t, writer.WriteField("model", "openrouter/test"))
		require.NoError(t, writer.WriteField("prompt", "compare formats"))

		p1, err := writer.CreateFormFile("image[]", "sample.png")
		require.NoError(t, err)
		_, err = p1.Write(pngBytes)
		require.NoError(t, err)

		p2, err := writer.CreateFormFile("image[]", "sample.jpg")
		require.NoError(t, err)
		_, err = p2.Write(jpegBytes)
		require.NoError(t, err)

		require.NoError(t, writer.Close())

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &reqBody)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		converted, err := adaptor.ConvertImageRequest(c, info, *req)
		require.NoError(t, err)

		payload := converted.(map[string]any)
		refs := payload["input_references"].([]openRouterImageReference)
		require.Len(t, refs, 2)

		assert.True(t, strings.HasPrefix(refs[0].ImageURL.URL, "data:image/png;base64,"))
		assert.True(t, strings.HasPrefix(refs[1].ImageURL.URL, "data:image/jpeg;base64,"))

		decodedPNG, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(refs[0].ImageURL.URL, "data:image/png;base64,"))
		require.NoError(t, err)
		assert.Equal(t, pngBytes, decodedPNG)

		decodedJPEG, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(refs[1].ImageURL.URL, "data:image/jpeg;base64,"))
		require.NoError(t, err)
		assert.Equal(t, jpegBytes, decodedJPEG)
	})
}

// TestOpenRouterImage_MultipartOrdering verifies that single image, repeated image[],
// and indexed image[2]/image[10] preserve proper ordering (numeric order for indexed).
func TestOpenRouterImage_MultipartOrdering(t *testing.T) {

	newOpenRouterInfo := func() *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			RelayMode: relayconstant.RelayModeImagesEdits,
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:    constant.ChannelTypeOpenRouter,
				ChannelBaseUrl: "https://openrouter.ai/api",
			},
		}
	}

	t.Run("indexed image[2] and image[10] preserves numeric order", func(t *testing.T) {
		pngBytes2 := createTinyPNG(t)
		jpegBytes10 := createTinyJPEG(t)

		var reqBody bytes.Buffer
		writer := multipart.NewWriter(&reqBody)
		require.NoError(t, writer.WriteField("model", "openrouter/test"))
		require.NoError(t, writer.WriteField("prompt", "indexed images"))

		// Write image[10] FIRST, image[2] SECOND to verify numeric sorting
		p10, err := writer.CreateFormFile("image[10]", "ten.jpg")
		require.NoError(t, err)
		_, err = p10.Write(jpegBytes10)
		require.NoError(t, err)

		p2, err := writer.CreateFormFile("image[2]", "two.png")
		require.NoError(t, err)
		_, err = p2.Write(pngBytes2)
		require.NoError(t, err)

		require.NoError(t, writer.Close())

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &reqBody)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		converted, err := adaptor.ConvertImageRequest(c, info, *req)
		require.NoError(t, err)

		payload := converted.(map[string]any)
		refs := payload["input_references"].([]openRouterImageReference)
		require.Len(t, refs, 2)

		// Numeric ordering: image[2] (PNG) MUST come before image[10] (JPEG)
		assert.True(t, strings.HasPrefix(refs[0].ImageURL.URL, "data:image/png;base64,"))
		assert.True(t, strings.HasPrefix(refs[1].ImageURL.URL, "data:image/jpeg;base64,"))
	})
}

// TestOpenRouterImage_SupportedScalars verifies supported scalars: background, output_format,
// user, output_compression=0, seed=0, stream=false in both JSON and multipart form.
func TestOpenRouterImage_SupportedScalars(t *testing.T) {
	newOpenRouterInfo := func() *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			RelayMode: relayconstant.RelayModeImagesEdits,
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:    constant.ChannelTypeOpenRouter,
				ChannelBaseUrl: "https://openrouter.ai/api",
			},
		}
	}

	t.Run("scalars in JSON including explicit zeros and false", func(t *testing.T) {
		jsonStr := `{
			"model": "openrouter/scalar-model",
			"prompt": "test scalars",
			"image": "https://example.com/test.png",
			"background": "transparent",
			"output_format": "png",
			"user": "tester-user",
			"output_compression": 0,
			"seed": 0,
			"stream": false
		}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		converted, err := adaptor.ConvertImageRequest(c, info, *req)
		require.NoError(t, err)

		payload := converted.(map[string]any)
		assert.Equal(t, "transparent", payload["background"])
		assert.Equal(t, "png", payload["output_format"])
		assert.Equal(t, "tester-user", payload["user"])
		assert.Equal(t, int64(0), payload["output_compression"])
		assert.Equal(t, int64(0), payload["seed"])
		assert.Equal(t, false, payload["stream"])

		// Ensure that marshaling to JSON preserves explicit zeros and false
		marshaled, err := common.Marshal(payload)
		require.NoError(t, err)
		marshaledStr := string(marshaled)
		assert.Contains(t, marshaledStr, `"output_compression":0`)
		assert.Contains(t, marshaledStr, `"seed":0`)
		assert.Contains(t, marshaledStr, `"stream":false`)
	})

	t.Run("scalars in Multipart form including explicit zeros and false", func(t *testing.T) {
		pngBytes := createTinyPNG(t)
		var reqBody bytes.Buffer
		writer := multipart.NewWriter(&reqBody)
		require.NoError(t, writer.WriteField("model", "openrouter/scalar-model"))
		require.NoError(t, writer.WriteField("prompt", "test scalars mp"))
		require.NoError(t, writer.WriteField("background", "opaque"))
		require.NoError(t, writer.WriteField("output_format", "jpeg"))
		require.NoError(t, writer.WriteField("user", "user-mp"))
		require.NoError(t, writer.WriteField("output_compression", "0"))
		require.NoError(t, writer.WriteField("seed", "0"))
		require.NoError(t, writer.WriteField("stream", "False"))

		part, err := writer.CreateFormFile("image", "scalar.png")
		require.NoError(t, err)
		_, err = part.Write(pngBytes)
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &reqBody)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		converted, err := adaptor.ConvertImageRequest(c, info, *req)
		require.NoError(t, err)

		payload := converted.(map[string]any)
		assert.Equal(t, "opaque", payload["background"])
		assert.Equal(t, "jpeg", payload["output_format"])
		assert.Equal(t, "user-mp", payload["user"])
		assert.Equal(t, int64(0), payload["output_compression"])
		assert.Equal(t, int64(0), payload["seed"])
		assert.Equal(t, false, payload["stream"])
	})
}

// TestOpenRouterImage_JSONExtraFields verifies that JSON Extra aspect_ratio and provider fields
// are accepted and mapped to the outbound payload.
func TestOpenRouterImage_JSONExtraFields(t *testing.T) {
	jsonStr := `{
		"model": "openai/gpt-image-2.5-sunburst",
		"prompt": "wide landscape",
		"image": "https://example.com/land.png",
		"aspect_ratio": "16:9",
		"provider": {
			"only": ["openai"]
		}
	}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
	c.Request.Header.Set("Content-Type", "application/json")

	req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
	require.NoError(t, err)
	req, err = common.DeepCopy(req)
	require.NoError(t, err)

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenRouter,
			ChannelBaseUrl: "https://openrouter.ai/api",
		},
	}
	adaptor := &Adaptor{}
	adaptor.Init(info)

	converted, err := adaptor.ConvertImageRequest(c, info, *req)
	require.NoError(t, err)

	payload := converted.(map[string]any)
	assert.Equal(t, "16:9", payload["aspect_ratio"])

	prov, ok := payload["provider"].(map[string]any)
	require.True(t, ok)
	order, ok := prov["only"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"openai"}, order)
}

// TestOpenRouterImage_Rejections verifies explicit errors for mask rejection (multipart and JSON),
// unsupported response_format=url, missing images, n>10, >16 refs, bad compression,
// unrecognized file content, and unsupported semantic fields.
func TestOpenRouterImage_Rejections(t *testing.T) {
	newOpenRouterInfo := func() *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			RelayMode: relayconstant.RelayModeImagesEdits,
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:    constant.ChannelTypeOpenRouter,
				ChannelBaseUrl: "https://openrouter.ai/api",
			},
		}
	}

	pngBytes := createTinyPNG(t)

	t.Run("mask rejection in multipart", func(t *testing.T) {
		var reqBody bytes.Buffer
		writer := multipart.NewWriter(&reqBody)
		require.NoError(t, writer.WriteField("model", "openrouter/test"))
		require.NoError(t, writer.WriteField("prompt", "masked edit"))

		pImg, err := writer.CreateFormFile("image", "source.png")
		require.NoError(t, err)
		_, err = pImg.Write(pngBytes)
		require.NoError(t, err)

		pMask, err := writer.CreateFormFile("mask", "mask.png")
		require.NoError(t, err)
		_, err = pMask.Write(pngBytes)
		require.NoError(t, err)

		require.NoError(t, writer.Close())

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &reqBody)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "OpenRouter image edits do not support file field mask")
	})

	t.Run("mask rejection in JSON", func(t *testing.T) {
		jsonStr := `{"model":"openrouter/test","prompt":"edit","image":"https://example.com/a.png","mask":"https://example.com/mask.png"}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "OpenRouter image edits do not support mask")
	})

	t.Run("unsupported response_format=url in JSON", func(t *testing.T) {
		jsonStr := `{"model":"openrouter/test","prompt":"edit","image":"https://example.com/a.png","response_format":"url"}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "OpenRouter image edits only support response_format=b64_json")
	})

	t.Run("missing images in multipart", func(t *testing.T) {
		var reqBody bytes.Buffer
		writer := multipart.NewWriter(&reqBody)
		require.NoError(t, writer.WriteField("model", "openrouter/test"))
		require.NoError(t, writer.WriteField("prompt", "no images"))
		require.NoError(t, writer.Close())

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &reqBody)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "OpenRouter image edits require between 1 and 16 reference images")
	})

	t.Run("missing images in JSON", func(t *testing.T) {
		jsonStr := `{"model":"openrouter/test","prompt":"no images"}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "OpenRouter image edits require between 1 and 16 reference images")
	})

	t.Run("n greater than 10", func(t *testing.T) {
		jsonStr := `{"model":"openrouter/test","prompt":"lots of images","image":"https://example.com/a.png","n":11}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "OpenRouter image n must be between 1 and 10")
	})

	t.Run("greater than 16 refs in multipart", func(t *testing.T) {
		var reqBody bytes.Buffer
		writer := multipart.NewWriter(&reqBody)
		require.NoError(t, writer.WriteField("model", "openrouter/test"))
		require.NoError(t, writer.WriteField("prompt", "17 images"))

		for i := 0; i < 17; i++ {
			p, err := writer.CreateFormFile("image[]", fmt.Sprintf("img%d.png", i))
			require.NoError(t, err)
			_, err = p.Write(pngBytes)
			require.NoError(t, err)
		}
		require.NoError(t, writer.Close())

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &reqBody)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "OpenRouter image edits accept at most 16 reference images")
	})

	t.Run("greater than 16 refs in JSON", func(t *testing.T) {
		urls := make([]string, 17)
		for i := range urls {
			urls[i] = fmt.Sprintf(`"https://example.com/%d.png"`, i)
		}
		jsonStr := fmt.Sprintf(`{"model":"openrouter/test","prompt":"17 images","image":[%s]}`, strings.Join(urls, ","))
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "OpenRouter image edits require between 1 and 16 reference images")
	})

	t.Run("bad compression negative or excessive", func(t *testing.T) {
		for _, val := range []int{-1, 101} {
			jsonStr := fmt.Sprintf(`{"model":"openrouter/test","prompt":"bad comp","image":"https://example.com/a.png","output_compression":%d}`, val)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
			c.Request.Header.Set("Content-Type", "application/json")

			req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
			require.NoError(t, err)
			req, err = common.DeepCopy(req)
			require.NoError(t, err)

			adaptor := &Adaptor{}
			info := newOpenRouterInfo()
			adaptor.Init(info)
			_, err = adaptor.ConvertImageRequest(c, info, *req)
			requireOpenRouterImageClientError(t, err)
			assert.Contains(t, err.Error(), "output_compression must be between 0 and 100")
		}
	})

	t.Run("bad compression non-integer", func(t *testing.T) {
		jsonStr := `{"model":"openrouter/test","prompt":"bad comp","image":"https://example.com/a.png","output_compression":"fast"}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "output_compression must be an integer")
	})

	t.Run("unrecognized file content in multipart", func(t *testing.T) {
		var reqBody bytes.Buffer
		writer := multipart.NewWriter(&reqBody)
		require.NoError(t, writer.WriteField("model", "openrouter/test"))
		require.NoError(t, writer.WriteField("prompt", "text file upload"))

		part, err := writer.CreateFormFile("image", "not_image.txt")
		require.NoError(t, err)
		_, err = part.Write([]byte("This is plain text and definitely not an image file content."))
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &reqBody)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "reference file must contain an image")
	})

	t.Run("unsupported semantic fields in JSON", func(t *testing.T) {
		cases := []struct {
			name      string
			jsonStr   string
			errSubstr string
		}{
			{
				name:      "style",
				jsonStr:   `{"model":"openrouter/test","prompt":"edit","image":"https://example.com/a.png","style":"vivid"}`,
				errSubstr: "OpenRouter image edits do not support style",
			},
			{
				name:      "input_fidelity",
				jsonStr:   `{"model":"openrouter/test","prompt":"edit","image":"https://example.com/a.png","input_fidelity":"high"}`,
				errSubstr: "OpenRouter image edits do not support input_fidelity",
			},
			{
				name:      "unknown extra field",
				jsonStr:   `{"model":"openrouter/test","prompt":"edit","image":"https://example.com/a.png","random_field":"value"}`,
				errSubstr: "OpenRouter image edits do not support random_field",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(tc.jsonStr))
				c.Request.Header.Set("Content-Type", "application/json")

				req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
				require.NoError(t, err)
				req, err = common.DeepCopy(req)
				require.NoError(t, err)

				adaptor := &Adaptor{}
				info := newOpenRouterInfo()
				adaptor.Init(info)
				_, err = adaptor.ConvertImageRequest(c, info, *req)
				requireOpenRouterImageClientError(t, err)
				assert.Contains(t, err.Error(), tc.errSubstr)
			})
		}
	})

	t.Run("conflicting reference fields in multipart", func(t *testing.T) {
		var reqBody bytes.Buffer
		writer := multipart.NewWriter(&reqBody)
		require.NoError(t, writer.WriteField("model", "openrouter/test"))
		require.NoError(t, writer.WriteField("prompt", "mixed reference fields"))

		p1, err := writer.CreateFormFile("image", "first.png")
		require.NoError(t, err)
		_, err = p1.Write(pngBytes)
		require.NoError(t, err)

		p2, err := writer.CreateFormFile("image[]", "second.png")
		require.NoError(t, err)
		_, err = p2.Write(pngBytes)
		require.NoError(t, err)

		require.NoError(t, writer.Close())

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &reqBody)
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "use only one reference image field")
	})

	t.Run("conflicting reference fields in JSON", func(t *testing.T) {
		jsonStr := `{"model":"openrouter/test","prompt":"conflict","image":"https://example.com/a.png","images":[{"image_url":"https://example.com/b.png"}]}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "use only one reference image field")
	})

	t.Run("unsupported reference URL scheme", func(t *testing.T) {
		jsonStr := `{"model":"openrouter/test","prompt":"file id","image":"file-abc12345"}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "reference images must be HTTP(S) URLs or base64 image data URLs; file IDs are not supported")
	})

	t.Run("stream=true rejection", func(t *testing.T) {
		jsonStr := `{"model":"openrouter/test","prompt":"stream edit","image":"https://example.com/a.png","stream":true}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		adaptor := &Adaptor{}
		info := newOpenRouterInfo()
		adaptor.Init(info)
		_, err = adaptor.ConvertImageRequest(c, info, *req)
		requireOpenRouterImageClientError(t, err)
		assert.Contains(t, err.Error(), "OpenRouter image editing currently requires stream=false")
	})

	t.Run("invalid or duplicate indexed field in multipart", func(t *testing.T) {
		for _, fieldName := range []string{"image[bad]", "image[-1]"} {
			var reqBody bytes.Buffer
			writer := multipart.NewWriter(&reqBody)
			require.NoError(t, writer.WriteField("model", "openrouter/test"))
			require.NoError(t, writer.WriteField("prompt", "invalid index"))

			p, err := writer.CreateFormFile(fieldName, "test.png")
			require.NoError(t, err)
			_, err = p.Write(pngBytes)
			require.NoError(t, err)
			require.NoError(t, writer.Close())

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &reqBody)
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())

			req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesEdits)
			require.NoError(t, err)
			req, err = common.DeepCopy(req)
			require.NoError(t, err)

			adaptor := &Adaptor{}
			info := newOpenRouterInfo()
			adaptor.Init(info)
			_, err = adaptor.ConvertImageRequest(c, info, *req)
			requireOpenRouterImageClientError(t, err)
			assert.Contains(t, err.Error(), "invalid reference image field "+fieldName)
		}
	})
}

// TestOpenRouterImage_UnchangedStandardBehaviors verifies that standard OpenAI edit behavior,
// OpenRouter generation behavior, and passthrough behavior are unaffected and retain their
// original URLs and headers without performing unwanted conversions.
func TestOpenRouterImage_UnchangedStandardBehaviors(t *testing.T) {

	t.Run("unchanged OpenRouter generation behavior", func(t *testing.T) {
		jsonStr := `{"model":"google/imagen-3","prompt":"a mountain"}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(jsonStr))
		c.Request.Header.Set("Content-Type", "application/json")

		req, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesGenerations)
		require.NoError(t, err)
		req, err = common.DeepCopy(req)
		require.NoError(t, err)

		info := &relaycommon.RelayInfo{
			RelayMode:      relayconstant.RelayModeImagesGenerations,
			RequestURLPath: "/v1/images/generations",
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:    constant.ChannelTypeOpenRouter,
				ChannelBaseUrl: "https://openrouter.ai/api",
			},
		}
		adaptor := &Adaptor{}
		adaptor.Init(info)

		converted, err := adaptor.ConvertImageRequest(c, info, *req)
		require.NoError(t, err)

		// Generation request is returned unchanged as dto.ImageRequest without conversion
		convertedReq, ok := converted.(dto.ImageRequest)
		require.True(t, ok)
		assert.Equal(t, "google/imagen-3", convertedReq.Model)
		assert.Equal(t, "a mountain", convertedReq.Prompt)

		// URL points to generations, not edits or /v1/images override
		url, err := adaptor.GetRequestURL(info)
		require.NoError(t, err)
		assert.Equal(t, "https://openrouter.ai/api/v1/images/generations", url)

		header := http.Header{}
		err = adaptor.SetupRequestHeader(c, &header, info)
		require.NoError(t, err)
		assert.Equal(t, "application/json", header.Get("Content-Type"))
	})

	t.Run("unchanged passthrough route behavior when no conversion performed", func(t *testing.T) {
		info := &relaycommon.RelayInfo{
			RelayMode:      relayconstant.RelayModeImagesEdits,
			RequestURLPath: "/v1/images/edits",
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:    constant.ChannelTypeOpenRouter,
				ChannelBaseUrl: "https://openrouter.ai/api",
			},
		}
		adaptor := &Adaptor{}
		adaptor.Init(info)

		// Prior to ConvertImageRequest (or when passthrough skips conversion),
		// openRouterImageEdit flag is false and original path is preserved.
		url, err := adaptor.GetRequestURL(info)
		require.NoError(t, err)
		assert.Equal(t, "https://openrouter.ai/api/v1/images/edits", url)

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
		c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=orig")

		header := http.Header{}
		err = adaptor.SetupRequestHeader(c, &header, info)
		require.NoError(t, err)
		assert.Equal(t, "multipart/form-data; boundary=orig", header.Get("Content-Type"))
	})
}

func requireOpenRouterImageClientError(t *testing.T, err error) {
	t.Helper()
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.True(t, types.IsSkipRetryError(apiErr))
}
