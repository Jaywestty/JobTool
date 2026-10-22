package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// OpenAICompatible works with any endpoint exposing POST /v1/chat/completions,
// including Ollama's compatibility API and hosted providers.
type OpenAICompatible struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenAICompatible(baseURL, apiKey, model string) *OpenAICompatible {
	return &OpenAICompatible{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (o *OpenAICompatible) Name() string { return "openai_compatible" }

const extractionPrompt = `Extract structured fields from the text. Reply with JSON only:
{"skills":[string],"years":number,"seniority":string,"role_family":string,"location":string,"remote_scope":string}`

func (o *OpenAICompatible) Extract(ctx context.Context, kind, text string) (Extraction, error) {
	body := map[string]any{
		"model": o.model,
		"messages": []map[string]string{
			{"role": "system", "content": extractionPrompt},
			{"role": "user", "content": "Kind: " + kind + "\n\n" + text},
		},
		"temperature": 0,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Extraction{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return Extraction{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return Extraction{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return Extraction{}, parseError(resp.StatusCode, string(raw))
	}

	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Extraction{}, err
	}
	if len(envelope.Choices) == 0 {
		return Extraction{}, errors.New("llm provider returned no choices")
	}
	return ParseExtraction(envelope.Choices[0].Message.Content)
}

// ParseExtraction tolerates a JSON code fence around the model output.
func ParseExtraction(content string) (Extraction, error) {
	s := content
	if i := indexOf(s, "{"); i >= 0 {
		if j := lastIndexOf(s, "}"); j >= i {
			s = s[i : j+1]
		}
	}
	var out Extraction
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return Extraction{}, err
	}
	return out, nil
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func lastIndexOf(s, sub string) int {
	for i := len(s) - len(sub); i >= 0; i-- {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
