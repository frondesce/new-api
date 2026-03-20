package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	miMoTTSModelPrefix     = "mimo-v2-tts"
	miMoDefaultVoice       = "mimo_default"
	miMoDefaultAudioFormat = "mp3"
	miMoTTSSynthesisPrompt = "Please synthesize the following assistant message into speech."
)

type miMoTTSChatResponse struct {
	Choices []miMoTTSChatChoice `json:"choices"`
	Usage   *dto.Usage          `json:"usage,omitempty"`
	Error   any                 `json:"error,omitempty"`
	Audio   json.RawMessage     `json:"audio,omitempty"`
}

type miMoTTSChatChoice struct {
	Message miMoTTSChatMessage `json:"message"`
	Audio   json.RawMessage    `json:"audio,omitempty"`
}

type miMoTTSChatMessage struct {
	Audio   json.RawMessage `json:"audio,omitempty"`
	Content any             `json:"content,omitempty"`
}

type miMoTTSAudioPayload struct {
	Data     string `json:"data,omitempty"`
	Audio    string `json:"audio,omitempty"`
	Format   string `json:"format,omitempty"`
	URL      string `json:"url,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

func isMiMoChatTTSModel(model string) bool {
	normalized := strings.TrimSpace(strings.ToLower(model))
	return strings.HasPrefix(normalized, miMoTTSModelPrefix)
}

func shouldUseMiMoTTSAudioBridge(info *relaycommon.RelayInfo) bool {
	return info != nil &&
		info.RelayMode == relayconstant.RelayModeAudioSpeech &&
		isMiMoChatTTSModel(info.UpstreamModelName)
}

func buildMiMoTTSChatRequest(info *relaycommon.RelayInfo, request dto.AudioRequest) ([]byte, error) {
	if info != nil && info.IsStream {
		return nil, errors.New("mimo-v2-tts does not support /v1/audio/speech streaming compatibility")
	}

	input := applyMiMoSpeedStyle(request.Input, request.Speed)

	audioPayload := map[string]any{
		"format": mapMiMoRequestAudioFormat(request.ResponseFormat),
		"voice":  mapMiMoVoice(request.Voice),
	}

	body := map[string]any{
		"model":    request.Model,
		"messages": buildMiMoMessages(request.Instructions, input),
		"audio":    audioPayload,
	}

	if len(request.Metadata) > 0 {
		var metadata map[string]any
		if err := common.Unmarshal(request.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("error unmarshalling metadata to mimo request: %w", err)
		}
		mergeMiMoRequestMap(body, metadata)
	}

	return common.Marshal(body)
}

func buildMiMoMessages(instructions, input string) []dto.Message {
	prompt := miMoTTSSynthesisPrompt
	if trimmedInstructions := strings.TrimSpace(instructions); trimmedInstructions != "" {
		prompt += " Follow these speaking instructions: " + trimmedInstructions
	}

	return []dto.Message{
		{
			Role:    "user",
			Content: prompt,
		},
		{
			Role:    "assistant",
			Content: input,
		},
	}
}

func applyMiMoSpeedStyle(input string, speed *float64) string {
	style := mapMiMoSpeedStyle(speed)
	if style == "" {
		return input
	}

	trimmed := strings.TrimSpace(input)
	lowerTrimmed := strings.ToLower(trimmed)
	if strings.HasPrefix(lowerTrimmed, "<style>") {
		closeTagIdx := strings.Index(lowerTrimmed, "</style>")
		if closeTagIdx > len("<style>") {
			currentStyles := strings.TrimSpace(trimmed[len("<style>"):closeTagIdx])
			if strings.Contains(strings.ToLower(currentStyles), strings.ToLower(style)) {
				return input
			}
			updatedStyles := strings.TrimSpace(currentStyles + " " + style)
			return "<style>" + updatedStyles + "</style>" + trimmed[closeTagIdx+len("</style>"):]
		}
	}

	return "<style>" + style + "</style>" + input
}

func mapMiMoSpeedStyle(speed *float64) string {
	if speed == nil {
		return ""
	}

	value := *speed
	switch {
	case math.IsNaN(value), math.IsInf(value, 0), value <= 0:
		return ""
	case value > 1:
		return "Speed up"
	case value < 1:
		return "Slow down"
	default:
		return ""
	}
}

func mergeMiMoRequestMap(target, source map[string]any) {
	for key, value := range source {
		sourceMap, sourceIsMap := value.(map[string]any)
		if !sourceIsMap {
			target[key] = value
			continue
		}

		targetMap, targetIsMap := target[key].(map[string]any)
		if !targetIsMap {
			target[key] = value
			continue
		}

		mergeMiMoRequestMap(targetMap, sourceMap)
	}
}

func mapMiMoVoice(voice string) string {
	switch strings.TrimSpace(strings.ToLower(voice)) {
	case "", "alloy", "echo", "fable", "onyx", "nova", "shimmer":
		return miMoDefaultVoice
	default:
		return voice
	}
}

func mapMiMoRequestAudioFormat(format string) string {
	switch strings.TrimSpace(strings.ToLower(format)) {
	case "", miMoDefaultAudioFormat:
		return miMoDefaultAudioFormat
	case "pcm":
		return "pcm16"
	default:
		return format
	}
}

func normalizeMiMoResponseAudioFormat(requestFormat, responseFormat, mimeType string) string {
	switch strings.TrimSpace(strings.ToLower(responseFormat)) {
	case "":
		if inferredFormat := inferMiMoAudioFormatFromMimeType(mimeType); inferredFormat != "" {
			return inferredFormat
		}
		if strings.TrimSpace(requestFormat) == "" {
			return miMoDefaultAudioFormat
		}
		return strings.ToLower(requestFormat)
	case "pcm16":
		return "pcm"
	default:
		return strings.ToLower(responseFormat)
	}
}

func inferMiMoAudioFormatFromMimeType(mimeType string) string {
	switch strings.TrimSpace(strings.ToLower(mimeType)) {
	case "audio/wav", "audio/x-wav":
		return "wav"
	case "audio/ogg", "audio/opus":
		return "ogg"
	case "audio/pcm", "audio/l16":
		return "pcm"
	case "audio/aac":
		return "aac"
	case "audio/flac":
		return "flac"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	default:
		return ""
	}
}

func getMiMoAudioContentType(format string) string {
	switch strings.TrimSpace(strings.ToLower(format)) {
	case "wav":
		return "audio/wav"
	case "ogg", "opus", "ogg_opus":
		return "audio/ogg"
	case "pcm", "pcm16":
		return "audio/pcm"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "mp3", "":
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}

func MiMoTTSHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var miMoResp miMoTTSChatResponse
	if err = common.Unmarshal(responseBody, &miMoResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := dto.GetOpenAIError(miMoResp.Error); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	audioPayload, err := extractMiMoTTSAudioPayload(&miMoResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if audioPayload.URL != "" {
		c.Redirect(http.StatusFound, audioPayload.URL)
		return buildMiMoTTSUsage(c, info, miMoResp.Usage, nil, miMoDefaultAudioFormat), nil
	}

	audioBase64 := audioPayload.Data
	if audioBase64 == "" {
		audioBase64 = audioPayload.Audio
	}
	if audioBase64 == "" {
		return nil, types.NewOpenAIError(errors.New("no audio data in mimo tts response"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	audioBase64, err = service.DecodeBase64AudioData(audioBase64)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	audioBytes, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	requestFormat := miMoDefaultAudioFormat
	if audioReq, ok := info.Request.(*dto.AudioRequest); ok && audioReq.ResponseFormat != "" {
		requestFormat = audioReq.ResponseFormat
	}
	audioFormat := normalizeMiMoResponseAudioFormat(requestFormat, audioPayload.Format, audioPayload.MimeType)
	contentType := getMiMoAudioContentType(audioFormat)

	c.Data(http.StatusOK, contentType, audioBytes)

	return buildMiMoTTSUsage(c, info, miMoResp.Usage, audioBytes, audioFormat), nil
}

func extractMiMoTTSAudioPayload(response *miMoTTSChatResponse) (*miMoTTSAudioPayload, error) {
	if response == nil {
		return nil, errors.New("empty mimo tts response")
	}

	candidates := make([]json.RawMessage, 0, 3)
	if len(response.Choices) > 0 {
		if len(response.Choices[0].Message.Audio) > 0 {
			candidates = append(candidates, response.Choices[0].Message.Audio)
		}
		if len(response.Choices[0].Audio) > 0 {
			candidates = append(candidates, response.Choices[0].Audio)
		}
	}
	if len(response.Audio) > 0 {
		candidates = append(candidates, response.Audio)
	}

	for _, candidate := range candidates {
		audioPayload, err := parseMiMoTTSAudioPayload(candidate)
		if err == nil && audioPayload != nil &&
			(audioPayload.Data != "" || audioPayload.Audio != "" || audioPayload.URL != "") {
			return audioPayload, nil
		}
	}

	return nil, errors.New("no audio payload found in mimo tts response")
}

func parseMiMoTTSAudioPayload(raw json.RawMessage) (*miMoTTSAudioPayload, error) {
	if len(raw) == 0 {
		return nil, errors.New("empty audio payload")
	}

	audioPayload := &miMoTTSAudioPayload{}
	if err := common.Unmarshal(raw, audioPayload); err == nil &&
		(audioPayload.Data != "" || audioPayload.Audio != "" || audioPayload.URL != "" || audioPayload.Format != "") {
		return audioPayload, nil
	}

	var audioString string
	if err := common.Unmarshal(raw, &audioString); err == nil && audioString != "" {
		return &miMoTTSAudioPayload{Data: audioString}, nil
	}

	return nil, errors.New("unsupported audio payload")
}

func buildMiMoTTSUsage(c *gin.Context, info *relaycommon.RelayInfo, upstreamUsage *dto.Usage, audioBytes []byte, audioFormat string) *dto.Usage {
	if upstreamUsage != nil &&
		(upstreamUsage.TotalTokens != 0 || upstreamUsage.PromptTokens != 0 || upstreamUsage.CompletionTokens != 0 ||
			upstreamUsage.InputTokens != 0 || upstreamUsage.OutputTokens != 0) {
		usage := *upstreamUsage
		if usage.PromptTokens == 0 {
			usage.PromptTokens = usage.InputTokens
		}
		if usage.CompletionTokens == 0 {
			usage.CompletionTokens = usage.OutputTokens
		}
		if usage.TotalTokens == 0 {
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		if usage.PromptTokensDetails.TextTokens == 0 && usage.PromptTokens > 0 {
			usage.PromptTokensDetails.TextTokens = usage.PromptTokens
		}
		if usage.CompletionTokenDetails.AudioTokens == 0 && usage.CompletionTokens > 0 {
			usage.CompletionTokenDetails.AudioTokens = usage.CompletionTokens
		}
		return &usage
	}

	common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
	usage := &dto.Usage{}
	usage.PromptTokens = info.GetEstimatePromptTokens()
	usage.TotalTokens = usage.PromptTokens
	usage.PromptTokensDetails.TextTokens = usage.PromptTokens

	if len(audioBytes) == 0 {
		return usage
	}

	duration, durationErr := getMiMoTTSAudioDuration(audioBytes, audioFormat)
	if durationErr != nil {
		sizeInKB := float64(len(audioBytes)) / 1000.0
		estimatedTokens := int(math.Ceil(sizeInKB))
		usage.CompletionTokens = estimatedTokens
		usage.CompletionTokenDetails.AudioTokens = estimatedTokens
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		return usage
	}

	if duration > 0 {
		completionTokens := int(math.Round(math.Ceil(duration) / 60.0 * 1000))
		usage.CompletionTokens = completionTokens
		usage.CompletionTokenDetails.AudioTokens = completionTokens
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return usage
}

func getMiMoTTSAudioDuration(audioBytes []byte, audioFormat string) (float64, error) {
	audioFormat = strings.TrimSpace(strings.ToLower(audioFormat))
	if audioFormat == "pcm" || audioFormat == "pcm16" {
		const sampleRate = 24000
		const bytesPerSample = 2
		const channels = 1
		return float64(len(audioBytes)) / float64(sampleRate*bytesPerSample*channels), nil
	}

	ext := "." + audioFormat
	return common.GetAudioDuration(context.Background(), bytes.NewReader(audioBytes), ext)
}
