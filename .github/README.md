# New API · Personal Fork

A personally maintained fork of [New API](https://github.com/QuantumNous/new-api), originally developed by [QuantumNous](https://github.com/QuantumNous), with focused improvements to provider compatibility, tool calling, image editing, and text-to-speech.

This page describes the changes maintained in this fork. For the full New API feature set and general setup instructions, see the [upstream README](../README.md), [简体中文说明](../README.zh_CN.md), and [official documentation](https://docs.newapi.ai/).

## Improvements in this fork

### Custom endpoints and speech

- **Use complete Gemini and Vertex AI request URLs.** Custom channels can explicitly select the OpenAI-compatible or Gemini / Vertex AI native protocol. Gemini URL templates support `{model}` and `{action}`, switching between ordinary and streaming requests automatically.
- **Call MiMo TTS through the OpenAI speech API.** The `/v1/audio/speech` bridge converts speech requests into MiMo's Chat TTS format and returns audio to compatible clients. It handles speaking instructions and converts voice and format options for non-streaming requests.

### Image editing with OpenRouter

- **Use OpenAI-compatible image-edit clients with OpenRouter.** Convert `/v1/images/edits` into OpenRouter's JSON image API, carrying reference images from uploaded files, URLs, or data URLs and applying channel model mappings.

This adapter supports non-streaming edits. Disable request-body passthrough on the OpenRouter channel to use the conversion. See the [image-editing guide and Open WebUI setup](../docs/openrouter-image-edits.md).

### Tool calling and Gemini search sources

- **Repair missing tool-call indexes from Cloudflare AI Gateway.** Streamed tool calls remain usable by OpenAI-compatible clients without losing provider extension fields, including Gemini thought signatures.
- **Continue Gemini tool conversations through Cloudflare.** Recover tool-result names from earlier calls and filter unnamed orphan results. Thought-signature compatibility follows the existing Gemini setting.
- **Accept legacy Google Search tool declarations.** Preserve `tools: [{"googleSearch": {}}]` when converting OpenAI-style requests to Gemini.
- **Preserve Gemini sources without text offsets.** Return available source links even when grounding segment offsets are missing. Missing citation titles fall back to the source domain or URL, without inventing a text range.

### User management

- **Keep deleted accounts out of ordinary user lists.** Default listing and search exclude soft-deleted users, while the explicit deleted-user filter remains available.

## Quick start

Use this fork's GHCR image for a local instance with SQLite:

```bash
docker run -d \
  --name new-api \
  -p 127.0.0.1:3000:3000 \
  -e TZ=Asia/Shanghai \
  -v new-api-data:/data \
  --restart unless-stopped \
  ghcr.io/frondesce/new-api:latest
```

On the Docker host, open [http://localhost:3000](http://localhost:3000) and complete the setup wizard to create the administrator account. The named volume persists application data across container replacements.

For a remote server, HTTPS, external databases, and other deployment options, follow the [deployment guide](../README.md#deployment). After configuring the repository's [Compose file](../docker-compose.yml), select this fork's image with:

```bash
NEW_API_IMAGE=ghcr.io/frondesce/new-api:latest docker compose up -d
```

The [GHCR workflow](workflows/docker-image-ghcr.yml) builds for `linux/amd64` and `linux/arm64`, with commit-based tags alongside `latest`. Image references in the upstream guides belong to upstream; use `ghcr.io/frondesce/new-api` to run this fork.

## Documentation and maintenance

- [Full New API overview and installation guide](../README.md)
- [简体中文项目说明](../README.zh_CN.md)
- [Official New API documentation](https://docs.newapi.ai/)
- [OpenRouter image-editing guide](../docs/openrouter-image-edits.md)
- [Upstream project — QuantumNous/new-api](https://github.com/QuantumNous/new-api)
- [License](../LICENSE) and [attribution notices](../NOTICE)

The root README follows the upstream version integrated into this branch. Fork-specific documentation lives here so upstream README updates can be merged with fewer conflicts.
