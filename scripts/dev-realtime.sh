#!/usr/bin/env bash
# ============================================================
# AgentPrimordia Realtime Voice - One-click launcher (macOS / Linux)
#
# Runs a real voice session with the `ap realtime voice` CLI.
# Path 1 (keyless): local faster-whisper + Piper endpoints via
#                   AP_ASR_URL / AP_TTS_URL (or --asr-url/--tts-url).
# Path 2 (key):     AP_LLM_API_KEY (or AP_OPENAI_API_KEY) with
#                   OpenAI endpoints (default URLs when unset).
# Falls back to mock with a clear message when neither is configured.
#
# Usage:
#   ./scripts/dev-realtime.sh
#   ./scripts/dev-realtime.sh --asr-url http://127.0.0.1:9000/v1/audio/transcriptions --tts-url http://127.0.0.1:5002/v1/audio/speech
# ============================================================

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGENT_DIR="$ROOT/agentprimordia"

ASR_URL="${AP_ASR_URL:-}"
TTS_URL="${AP_TTS_URL:-}"
VOICE="${AP_TTS_VOICE:-}"
MOCK=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --asr-url) ASR_URL="$2"; shift 2 ;;
    --tts-url) TTS_URL="$2"; shift 2 ;;
    --tts-voice) VOICE="$2"; shift 2 ;;
    --mock) MOCK=1; shift ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

if ! command -v go >/dev/null 2>&1; then
  echo "error: missing command: go (install Go 1.26+ first)" >&2
  exit 1
fi

ASR="mock"
TTS="mock"
ARGS=(run ./cmd/ap realtime voice)

if [[ "$MOCK" -eq 0 ]]; then
  KEY="${AP_LLM_API_KEY:-${AP_OPENAI_API_KEY:-}}"

  if [[ -n "$ASR_URL" || -n "$TTS_URL" || -n "$KEY" ]]; then
    if [[ -z "$ASR_URL" && -n "$KEY" ]]; then ASR_URL="https://api.openai.com/v1/audio/transcriptions"; fi
    if [[ -z "$TTS_URL" && -n "$KEY" ]]; then TTS_URL="https://api.openai.com/v1/audio/speech"; fi
    if [[ -z "$ASR_URL" || -z "$TTS_URL" ]]; then
      echo "!! Real mode needs both ASR and TTS endpoints:" >&2
      echo "   local : set AP_ASR_URL (faster-whisper) and AP_TTS_URL (Piper)" >&2
      echo "   cloud : set AP_LLM_API_KEY (defaults to OpenAI endpoints)" >&2
      echo "   falling back to mock..." >&2
    else
      ASR="openai"
      TTS="openai"
      ARGS+=(--asr=openai --tts=openai "--asr-url=$ASR_URL" "--tts-url=$TTS_URL")
      if [[ -n "$VOICE" ]]; then ARGS+=("--tts-voice=$VOICE"); fi
    fi
  else
    echo "No AP_ASR_URL/AP_TTS_URL/AP_LLM_API_KEY set - using mock (real mode hints below)." >&2
  fi
fi

echo "==> AgentPrimordia realtime voice launcher"
echo "    ASR: $ASR | TTS: $TTS"
if [[ "$ASR" == "mock" ]]; then
  echo ""
  echo "Real mode examples:"
  echo "  ./scripts/dev-realtime.sh --asr-url http://127.0.0.1:9000/v1/audio/transcriptions --tts-url http://127.0.0.1:5002/v1/audio/speech"
  echo "  AP_LLM_API_KEY=sk-... ./scripts/dev-realtime.sh"
  echo ""
fi

echo "==> Running: ap realtime voice ($ASR/$TTS)..."
cd "$AGENT_DIR"
exec go "${ARGS[@]}"
