package openai

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
)

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
