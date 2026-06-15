package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestValidateChannelRejectsActionPlaceholderForCustomOpenAIProtocol(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:    constant.ChannelTypeCustom,
		Key:     "test-key",
		BaseURL: common.GetPointer("https://gateway.example.com/models/{model}:{action}"),
		Models:  "gemini-future-model",
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		CustomProtocol: dto.CustomChannelProtocolOpenAI,
	})

	err := validateChannel(channel, true)

	require.ErrorContains(t, err, "Gemini / Vertex AI 原生")
}

func TestValidateChannelAcceptsActionPlaceholderForCustomGeminiVertexProtocol(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:    constant.ChannelTypeCustom,
		Key:     "test-key",
		BaseURL: common.GetPointer("https://gateway.example.com/models/{model}:{action}"),
		Models:  "gemini-future-model",
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		CustomProtocol: dto.CustomChannelProtocolGeminiVertex,
	})

	require.NoError(t, validateChannel(channel, true))
}
