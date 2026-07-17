package nutrition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"essen/internal/config"
)

// NutritionResult contains the nutritional breakdown returned by the LLM.
type NutritionResult struct {
	CaloriesKcal float64 `json:"calories_kcal"`
	ProteinG     float64 `json:"protein_g"`
	FatG         float64 `json:"fat_g"`
	CarbsG       float64 `json:"carbs_g"`
	Notes        string  `json:"notes"`
}

// OpenAI‑compatible chat message.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAI‑compatible chat completion request body.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

// OpenAI‑compatible chat completion response.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Lookup queries nutritional data using a multi-source strategy:
//  1. fatsecret.cn (scraped, no API key).
//  2. OpenFoodFacts database (free, no API key).
//  3. Fallback: LLM estimation.
//
// brand is optional (empty string ok).
// cfg must have a resolvable API key (required only for the LLM fallback).
func Lookup(food string, brand string, amount string, cfg config.LLMConfig) (*NutritionResult, error) {
	// 1. Try fatsecret.cn first (data is per serving).
	result, err := fatsecretLookup(food, brand)
	if err != nil {
		// Non-fatal: log warning and continue to next source.
		fmt.Fprintf(os.Stderr, "警告: fatsecret.cn 查询失败: %v，尝试 OpenFoodFacts\n", err)
	}
	if result != nil {
		// Scale by count when user specifies discrete quantities (e.g. "6个" => ×6).
		// fatsecret returns per-serving data, but the user may eat multiple items.
		scaleFatsecretResult(result, amount)
		result.Notes = "fatsecret.cn | " + result.Notes
		return result, nil
	}

	// 2. Try OpenFoodFacts.
	result, err = openFoodFactsLookup(food, brand)
	if err != nil {
		// Non-fatal: log warning and continue to LLM fallback.
		fmt.Fprintf(os.Stderr, "警告: OpenFoodFacts 查询失败: %v，使用 LLM 估算\n", err)
	}
	if result != nil {
		scaleResultForAmount(result, amount)
		return result, nil
	}

	// 3. Fall back to LLM.
	return llmLookup(food, brand, amount, cfg)
}

// llmLookup queries an OpenAI‑compatible LLM API for the nutritional
// breakdown of the given food and amount.
func llmLookup(food string, brand string, amount string, cfg config.LLMConfig) (*NutritionResult, error) {
	apiKey := config.ResolveAPIKey(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("API key 未设置，请运行 'essen config --api-key KEY' 设置")
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	url := baseURL + "/chat/completions"

	systemPrompt := `你是一个精确的营养数据库。根据给定的食物名称和份量，返回准确的营养信息。

仅返回以下格式的纯 JSON，不要包含其他文字或 markdown：
{
  "calories_kcal": <热量千卡数>,
  "protein_g": <蛋白质克数>,
  "fat_g": <脂肪克数>,
  "carbs_g": <碳水克数>,
  "notes": "<简短备注>"
}

准则：
- 对于常见中国食物，使用标准营养数据
- 如份量不明确，使用合理估算并在 notes 中说明
- 热量应大致满足：蛋白质×4 + 脂肪×9 + 碳水×4`

	foodDesc := food
	if brand != "" {
		foodDesc = brand + " " + food
	}
	userMsg := fmt.Sprintf("查询食物营养信息: %s, 份量: %s", foodDesc, amount)

	body := chatRequest{
		Model: cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMsg},
		},
		Temperature: 0.1,
		MaxTokens:   1024,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 API 失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB limit
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API 返回错误状态 %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("解析 API 响应失败: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("API 返回空响应")
	}

	content := chatResp.Choices[0].Message.Content

	var result NutritionResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// Try to extract JSON from markdown fences or embedded text.
		jsonStr := extractJSON(content)
		if jsonStr == "" {
			return nil, fmt.Errorf("解析营养数据失败: %w\n原始响应: %s", err, content)
		}
		if err2 := json.Unmarshal([]byte(jsonStr), &result); err2 != nil {
			return nil, fmt.Errorf("解析营养数据失败: %w\n原始响应: %s", err2, content)
		}
	}

	return &result, nil
}

// countUnits are units that represent discrete countable items.
// When a user specifies e.g. "6个", fatsecret's per-serving data should
// be multiplied by 6 (assuming 1 serving = 1 item).
var countUnits = []string{"个", "串", "只", "块", "片", "条", "粒", "颗", "枚", "根", "支"}

// scaleFatsecretResult multiplies nutrition values when the amount
// specifies a discrete count (e.g., "6个羊肉串" => ×6).
//
// fatsecret returns data per serving, but the serving unit may be "1个"
// while the user ate "6个". Weight/volume units (g, ml) cannot be safely
// scaled without knowing fatsecret's serving weight, so they are left as-is.
func scaleFatsecretResult(result *NutritionResult, amount string) {
	n := extractNumber(amount)
	if n <= 1 {
		return // 0 or 1 serving => nothing to scale
	}

	// Only scale for count-based units.
	hasCountUnit := false
	for _, u := range countUnits {
		if strings.Contains(amount, u) {
			hasCountUnit = true
			break
		}
	}
	if !hasCountUnit {
		return // weight/volume unit => can't scale without serving weight
	}

	result.CaloriesKcal *= n
	result.ProteinG *= n
	result.FatG *= n
	result.CarbsG *= n
	result.Notes += fmt.Sprintf(" (×%.0f)", n)
}

// extractJSON attempts to pull a valid JSON object out of an LLM response
// that may contain markdown fences or surrounding text.
func extractJSON(content string) string {
	content = strings.TrimSpace(content)

	// 1. Markdown code fence with language hint: ```json … ```
	for _, fence := range []string{"```json", "```"} {
		idx := strings.Index(content, fence)
		if idx >= 0 {
			start := idx + len(fence)
			// Skip to next line after opening fence.
			if nl := strings.IndexByte(content[start:], '\n'); nl >= 0 {
				start += nl + 1
			}
			if end := strings.Index(content[start:], "```"); end >= 0 {
				candidate := strings.TrimSpace(content[start : start+end])
				if candidate != "" {
					return candidate
				}
			}
		}
	}

	// 2. Brace‑matching fallback: first '{' … last '}'.
	start := strings.IndexByte(content, '{')
	end := strings.LastIndexByte(content, '}')
	if start >= 0 && end > start {
		return content[start : end+1]
	}

	return ""
}
