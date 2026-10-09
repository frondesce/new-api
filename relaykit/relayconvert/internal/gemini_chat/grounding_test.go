package geminichat

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroundingCitationsKeepSourceLinksAndTitles(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata string
		want     string
	}{
		{
			name:     "missing support offsets keeps unique sources",
			metadata: `{"groundingChunks":[{"web":{"uri":"https://example.com","domain":"example.com"}},{"web":{"uri":"https://example.com","title":"duplicate"}},{"web":{"uri":"https://other.com"}}]}`,
			want:     `[{"type":"url_citation","url_citation":{"url":"https://example.com","title":"example.com"}},{"type":"url_citation","url_citation":{"url":"https://other.com","title":"https://other.com"}}]`,
		},
		{
			name:     "supported citation keeps upstream offsets and domain fallback",
			metadata: `{"groundingChunks":[{"web":{"uri":"https://example.com","domain":"example.com"}}],"groundingSupports":[{"segment":{"startIndex":0,"endIndex":5,"text":"hello"},"groundingChunkIndices":[0]}]}`,
			want:     `[{"type":"url_citation","url_citation":{"url":"https://example.com","title":"example.com","start_index":0,"end_index":5}}]`,
		},
		{
			name:     "provider title takes priority over domain",
			metadata: `{"groundingChunks":[{"web":{"uri":"https://example.com","title":"Page title","domain":"example.com"}}]}`,
			want:     `[{"type":"url_citation","url_citation":{"url":"https://example.com","title":"Page title"}}]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metadata := &dto.GeminiGroundingMetadata{}
			require.NoError(t, kitutil.Unmarshal([]byte(tc.metadata), metadata))
			response := &dto.GeminiChatResponse{Candidates: []dto.GeminiChatCandidate{{
				Content:           dto.GeminiChatContent{Role: "model", Parts: []dto.GeminiPart{{Text: "hello"}}},
				GroundingMetadata: metadata,
			}}}
			converted := ResponseGeminiChat2OpenAI("response-test", 1, response)
			require.Len(t, converted.Choices, 1)
			assert.JSONEq(t, tc.want, string(converted.Choices[0].Message.Annotations))

			state := NewGeminiToChatStreamState("response-test", 1)
			chunks, err := state.ConvertChunk(response, "gemini-test", nil)
			require.NoError(t, err)
			require.Len(t, chunks, 1)
			require.Len(t, chunks[0].Choices, 1)
			assert.JSONEq(t, tc.want, string(chunks[0].Choices[0].Delta.Annotations))
		})
	}
}

func TestGroundingStreamDoesNotRepeatSourceOnlyCitations(t *testing.T) {
	metadata := &dto.GeminiGroundingMetadata{}
	require.NoError(t, kitutil.Unmarshal([]byte(`{"groundingChunks":[{"web":{"uri":"https://example.com","domain":"example.com"}}]}`), metadata))
	response := &dto.GeminiChatResponse{Candidates: []dto.GeminiChatCandidate{{
		Content:           dto.GeminiChatContent{Role: "model", Parts: []dto.GeminiPart{{Text: "hello"}}},
		GroundingMetadata: metadata,
	}}}
	state := NewGeminiToChatStreamState("response-test", 1)
	first, err := state.ConvertChunk(response, "gemini-test", nil)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.NotEmpty(t, first[0].Choices[0].Delta.Annotations)
	second, err := state.ConvertChunk(response, "gemini-test", nil)
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.Empty(t, second[0].Choices[0].Delta.Annotations)
}
