package openai

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

const cloudflareGatewayGeminiThoughtSignatureBypassValue = "context_engineering_is_the_way_to_go"

type toolCallIndexState struct {
	indexByChoice map[int]map[string]int
	nextByChoice  map[int]int
}

func newToolCallIndexState() *toolCallIndexState {
	return &toolCallIndexState{
		indexByChoice: make(map[int]map[string]int),
		nextByChoice:  make(map[int]int),
	}
}

func (s *toolCallIndexState) assign(choiceIndex int, tool *dto.ToolCallResponse, fallback int) {
	if tool == nil {
		return
	}

	if tool.Index != nil {
		idx := *tool.Index
		if tool.ID != "" {
			m := s.indexByChoice[choiceIndex]
			if m == nil {
				m = make(map[string]int)
				s.indexByChoice[choiceIndex] = m
			}
			m[tool.ID] = idx
		}
		if next := idx + 1; s.nextByChoice[choiceIndex] < next {
			s.nextByChoice[choiceIndex] = next
		}
		return
	}

	if tool.ID != "" {
		m := s.indexByChoice[choiceIndex]
		if m == nil {
			m = make(map[string]int)
			s.indexByChoice[choiceIndex] = m
		}
		if idx, ok := m[tool.ID]; ok {
			tool.SetIndex(idx)
			return
		}
		idx := s.nextByChoice[choiceIndex]
		s.nextByChoice[choiceIndex] = idx + 1
		m[tool.ID] = idx
		tool.SetIndex(idx)
		return
	}

	tool.SetIndex(fallback)
	if next := fallback + 1; s.nextByChoice[choiceIndex] < next {
		s.nextByChoice[choiceIndex] = next
	}
}

func shouldNormalizeCloudflareGatewayToolCallIndexes(info *relaycommon.RelayInfo) bool {
	if info == nil || info.RelayMode != relayconstant.RelayModeChatCompletions || !info.IsStream {
		return false
	}
	return strings.Contains(strings.ToLower(info.ChannelBaseUrl), "gateway.ai.cloudflare.com")
}

func shouldApplyCloudflareGatewayGeminiCompat(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) bool {
	if info == nil || request == nil || info.RelayMode != relayconstant.RelayModeChatCompletions {
		return false
	}
	if !strings.Contains(strings.ToLower(info.ChannelBaseUrl), "gateway.ai.cloudflare.com") {
		return false
	}
	modelName := strings.ToLower(strings.TrimSpace(request.Model))
	if modelName == "" {
		modelName = strings.ToLower(strings.TrimSpace(info.UpstreamModelName))
	}
	return strings.HasPrefix(modelName, "google-ai-studio/") ||
		strings.Contains(modelName, "/gemini") ||
		strings.HasPrefix(modelName, "gemini")
}

func shouldInjectCloudflareGatewayGeminiThoughtSignature(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) bool {
	if !shouldApplyCloudflareGatewayGeminiCompat(info, request) {
		return false
	}
	return model_setting.GetGeminiSettings().FunctionCallThoughtSignatureEnabled
}

func sanitizeCloudflareGatewayGeminiMessages(request *dto.GeneralOpenAIRequest) error {
	if request == nil {
		return nil
	}

	toolCallNames := make(map[string]string)
	sanitizedMessages := make([]dto.Message, 0, len(request.Messages))
	for idx := range request.Messages {
		message := request.Messages[idx]

		if len(message.ToolCalls) > 0 && (message.Role == "assistant" || message.Role == "model") {
			var toolCalls []dto.ToolCallRequest
			if err := common.Unmarshal(message.ToolCalls, &toolCalls); err != nil {
				return fmt.Errorf("unmarshal tool calls: %w", err)
			}
			for _, toolCall := range toolCalls {
				if toolCall.ID == "" || strings.TrimSpace(toolCall.Function.Name) == "" {
					continue
				}
				toolCallNames[toolCall.ID] = toolCall.Function.Name
			}
			sanitizedMessages = append(sanitizedMessages, message)
			continue
		}

		if message.Role != "tool" && message.Role != "function" {
			sanitizedMessages = append(sanitizedMessages, message)
			continue
		}

		name := ""
		if message.Name != nil {
			name = strings.TrimSpace(*message.Name)
		}
		if name == "" && strings.TrimSpace(message.ToolCallId) != "" {
			name = strings.TrimSpace(toolCallNames[message.ToolCallId])
		}
		if name == "" {
			continue
		}
		if message.Name == nil || strings.TrimSpace(*message.Name) == "" {
			resolvedName := name
			message.Name = &resolvedName
		}
		sanitizedMessages = append(sanitizedMessages, message)
	}

	request.Messages = sanitizedMessages
	return nil
}

func injectCloudflareGatewayGeminiThoughtSignature(request *dto.GeneralOpenAIRequest) error {
	if request == nil {
		return nil
	}
	for idx := range request.Messages {
		message := &request.Messages[idx]
		if len(message.ToolCalls) == 0 {
			continue
		}
		if message.Role != "assistant" && message.Role != "model" {
			continue
		}

		var toolCalls []map[string]any
		if err := common.Unmarshal(message.ToolCalls, &toolCalls); err != nil {
			return fmt.Errorf("unmarshal tool calls: %w", err)
		}

		modified := false
		for toolIdx := range toolCalls {
			toolCall := toolCalls[toolIdx]
			if len(toolCall) == 0 {
				continue
			}

			extraContent, _ := toolCall["extra_content"].(map[string]any)
			if extraContent == nil {
				extraContent = make(map[string]any)
				toolCall["extra_content"] = extraContent
			}

			googleExtra, _ := extraContent["google"].(map[string]any)
			if googleExtra == nil {
				googleExtra = make(map[string]any)
				extraContent["google"] = googleExtra
			}

			if thoughtSignature, ok := googleExtra["thought_signature"].(string); ok && strings.TrimSpace(thoughtSignature) != "" {
				continue
			}

			googleExtra["thought_signature"] = cloudflareGatewayGeminiThoughtSignatureBypassValue
			modified = true
		}

		if !modified {
			continue
		}

		toolCallsRaw, err := common.Marshal(toolCalls)
		if err != nil {
			return fmt.Errorf("marshal tool calls: %w", err)
		}
		message.ToolCalls = toolCallsRaw
	}
	return nil
}

func normalizeCloudflareGatewayToolCallIndexes(data string, state *toolCallIndexState) (string, error) {
	if data == "" || state == nil {
		return data, nil
	}

	var response dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal(common.StringToByteSlice(data), &response); err != nil {
		return data, err
	}

	modified := false
	for choiceIdx := range response.Choices {
		choiceKey := response.Choices[choiceIdx].Index
		for toolIdx := range response.Choices[choiceIdx].Delta.ToolCalls {
			tool := &response.Choices[choiceIdx].Delta.ToolCalls[toolIdx]
			if tool.Index == nil {
				modified = true
			}
			state.assign(choiceKey, tool, toolIdx)
		}
	}

	if !modified {
		return data, nil
	}

	out, err := common.Marshal(response)
	if err != nil {
		return data, err
	}
	return string(out), nil
}
