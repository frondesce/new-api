package openai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

type openRouterImageReference struct {
	Type     string             `json:"type"`
	ImageURL openRouterImageURL `json:"image_url"`
}

type openRouterImageURL struct {
	URL string `json:"url"`
}

// OpenRouter accepts reference images on its JSON Image API, not multipart
// /images/edits. Keep the inbound headers and body intact for channel retries.
// https://openrouter.ai/docs/guides/overview/multimodal/image-generation
func convertOpenRouterImageEdit(c *gin.Context, request dto.ImageRequest) (map[string]any, error) {
	if strings.TrimSpace(request.Model) == "" || strings.TrimSpace(request.Prompt) == "" {
		return nil, fmt.Errorf("model and prompt are required for OpenRouter image edits")
	}
	n := uint(1)
	if request.N != nil {
		n = *request.N
	}
	if n < 1 || n > 10 || n > dto.MaxImageN {
		return nil, fmt.Errorf("OpenRouter image n must be between 1 and 10")
	}
	fields := make(map[string]json.RawMessage)
	var references []openRouterImageReference
	if isJSONRequest(c) {
		body, err := common.Marshal(request)
		if err != nil {
			return nil, err
		}
		if err := common.Unmarshal(body, &fields); err != nil {
			return nil, err
		}
		for key, value := range request.Extra {
			if _, exists := fields[key]; !exists {
				fields[key] = value
			}
		}
	} else {
		form := c.Request.MultipartForm
		if form == nil {
			var err error
			form, err = common.ParseMultipartFormReusable(c)
			if err != nil {
				return nil, fmt.Errorf("failed to parse image edit form: %w", err)
			}
			c.Request.MultipartForm = form
		}
		for key, values := range form.Value {
			if len(values) != 1 {
				return nil, fmt.Errorf("OpenRouter image edit field %s must occur once", key)
			}
			switch key {
			case "stream":
				value, err := strconv.ParseBool(strings.TrimSpace(values[0]))
				if err != nil {
					return nil, fmt.Errorf("stream must be a boolean")
				}
				fields[key], _ = common.Marshal(value)
			case "n", "output_compression", "seed", "provider", "input_references":
				fields[key] = json.RawMessage(values[0])
			default:
				fields[key], _ = common.Marshal(values[0])
			}
		}
		files, err := openRouterImageEditFiles(form)
		if err != nil {
			return nil, err
		}
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open reference image: %w", err)
			}
			data, err := io.ReadAll(file)
			_ = file.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read reference image: %w", err)
			}
			mediaType := http.DetectContentType(data)
			if !strings.HasPrefix(mediaType, "image/") {
				return nil, fmt.Errorf("reference file must contain an image")
			}
			references = append(references, openRouterImageReference{
				Type:     "image_url",
				ImageURL: openRouterImageURL{URL: "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)},
			})
		}
	}

	payload := map[string]any{"model": request.Model, "prompt": request.Prompt}
	for key, raw := range fields {
		if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
			continue
		}
		switch key {
		case "model", "prompt", "n": // Use the validated, model-mapped request below.
		case "size", "quality", "background", "output_format", "user", "resolution", "aspect_ratio", "response_format":
			var value string
			if err := common.Unmarshal(raw, &value); err != nil {
				return nil, fmt.Errorf("%s must be a string", key)
			}
			if key == "response_format" {
				if value != "" && value != "b64_json" {
					return nil, fmt.Errorf("OpenRouter image edits only support response_format=b64_json")
				}
				continue
			}
			// OpenAI's auto size means no fixed dimensions. OpenRouter exposes
			// auto via aspect_ratio instead; omitting size keeps provider defaults.
			if value != "" && !(key == "size" && value == "auto") {
				payload[key] = value
			}
		case "output_compression", "seed":
			var value int64
			if err := common.Unmarshal(raw, &value); err != nil {
				return nil, fmt.Errorf("%s must be an integer", key)
			}
			if key == "output_compression" && (value < 0 || value > 100) {
				return nil, fmt.Errorf("output_compression must be between 0 and 100")
			}
			payload[key] = value
		case "stream":
			var value bool
			if err := common.Unmarshal(raw, &value); err != nil {
				return nil, fmt.Errorf("stream must be a boolean")
			}
			// The shared stream handler forwards event types unchanged. OpenRouter
			// emits image_generation.*, whereas OpenAI edits use image_edit.*.
			if value {
				return nil, fmt.Errorf("OpenRouter image editing currently requires stream=false")
			}
			payload[key] = value
		case "provider":
			var value map[string]any
			if err := common.Unmarshal(raw, &value); err != nil {
				return nil, fmt.Errorf("provider must be an object")
			}
			payload[key] = value
		case "image", "images":
			var urls []string
			var single string
			if common.Unmarshal(raw, &single) == nil {
				urls = []string{single}
			} else if common.Unmarshal(raw, &urls) != nil {
				// A failed slice decode may leave partially initialized entries.
				urls = nil
				var images []struct {
					ImageURL string `json:"image_url"`
				}
				if err := common.Unmarshal(raw, &images); err != nil {
					return nil, fmt.Errorf("%s must contain image URLs or base64 data URLs", key)
				}
				for _, image := range images {
					urls = append(urls, image.ImageURL)
				}
			}
			if len(references) > 0 {
				return nil, fmt.Errorf("use only one reference image field")
			}
			for _, imageURL := range urls {
				references = append(references, openRouterImageReference{Type: "image_url", ImageURL: openRouterImageURL{URL: imageURL}})
			}
		case "input_references":
			if len(references) > 0 {
				return nil, fmt.Errorf("use only one reference image field")
			}
			if err := common.Unmarshal(raw, &references); err != nil {
				return nil, fmt.Errorf("invalid input_references: %w", err)
			}
		default:
			return nil, fmt.Errorf("OpenRouter image edits do not support %s", key)
		}
	}
	payload["n"] = n
	if len(references) < 1 || len(references) > 16 {
		return nil, fmt.Errorf("OpenRouter image edits require between 1 and 16 reference images")
	}
	for _, reference := range references {
		imageURL := reference.ImageURL.URL
		parsed, err := url.Parse(imageURL)
		isHTTP := err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
		isData := strings.HasPrefix(imageURL, "data:image/") && strings.Contains(imageURL, ";base64,")
		if reference.Type != "image_url" || (!isHTTP && !isData) {
			return nil, fmt.Errorf("reference images must be HTTP(S) URLs or base64 image data URLs; file IDs are not supported")
		}
	}
	payload["input_references"] = references
	return payload, nil
}

// Keep reference order stable, including image[2] before image[10]. Reject
// masks and ambiguous mixed field names instead of silently changing edits.
func openRouterImageEditFiles(form *multipart.Form) ([]*multipart.FileHeader, error) {
	var files []*multipart.FileHeader
	indexed := make(map[int][]*multipart.FileHeader)
	var indices []int
	for name, entries := range form.File {
		switch {
		case name == "image" || name == "image[]":
			if len(files) > 0 {
				return nil, fmt.Errorf("use only one reference image field")
			}
			files = entries
		case strings.HasPrefix(name, "image[") && strings.HasSuffix(name, "]"):
			index, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "image["), "]"))
			if err != nil || index < 0 || indexed[index] != nil {
				return nil, fmt.Errorf("invalid reference image field %s", name)
			}
			indexed[index] = entries
			indices = append(indices, index)
		default:
			return nil, fmt.Errorf("OpenRouter image edits do not support file field %s", name)
		}
	}
	if len(indices) > 0 && len(files) > 0 {
		return nil, fmt.Errorf("use only one reference image field")
	}
	sort.Ints(indices)
	for _, index := range indices {
		files = append(files, indexed[index]...)
	}
	if len(files) > 16 {
		return nil, fmt.Errorf("OpenRouter image edits accept at most 16 reference images")
	}
	return files, nil
}
