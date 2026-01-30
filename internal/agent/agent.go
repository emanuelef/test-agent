package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/emanuelefumagalli/test-agent/internal/ollama"
	"github.com/emanuelefumagalli/test-agent/internal/weather"
)

// Config wires together the dependencies and runtime options for the agent.
type Config struct {
	LocationName   string
	ForecastDays   int
	Weather        weather.Forecaster
	Ollama         *ollama.Client
	TelegramToken  string
	TelegramChatID string
}

// Agent coordinates the weather fetch and Ollama summarization.
type Agent struct {
	cfg Config
}

// New returns a fully constructed Agent.
func New(cfg Config) *Agent {
	if cfg.ForecastDays <= 0 {
		cfg.ForecastDays = 15
	}
	return &Agent{cfg: cfg}
}

// Run executes one fetch-and-summarize pass.
func (a *Agent) Run(ctx context.Context) error {
	if a.cfg.Weather == nil {
		return fmt.Errorf("weather client is missing")
	}
	if a.cfg.Ollama == nil {
		return fmt.Errorf("ollama client is missing")
	}

	forecast, err := a.cfg.Weather.Fetch(ctx, a.cfg.ForecastDays)
	if err != nil {
		return fmt.Errorf("fetch forecast: %w", err)
	}

	location := fallbackLocation(a.cfg.LocationName)
	report := buildForecastTable(forecast)
	fmt.Printf("%d-day %s wind forecast (km/h):\n", len(forecast), location)
	fmt.Println(report)

	prompt := buildPrompt(location, forecast, report)
	fmt.Println("\nPrompt sent to Ollama:\n----------------------")
	fmt.Println(prompt)
	fmt.Println("----------------------")
	summary, err := a.cfg.Ollama.Generate(ctx, prompt)
	if err != nil {
		fmt.Printf("Ollama failed: %v\n", err)
		if a.cfg.TelegramToken != "" && a.cfg.TelegramChatID != "" {
			err2 := sendTelegramMessage(&a.cfg, report)
			if err2 != nil {
				fmt.Printf("Failed to send Telegram message: %v\n", err2)
				return fmt.Errorf("ollama summary: %w; telegram: %v", err, err2)
			}
			fmt.Println("Sent fallback wind table to Telegram.")
			return nil
		}
		return fmt.Errorf("ollama summary: %w", err)
	}

	fmt.Println("\nOllama summary:")
	fmt.Println(summary)

	// Send to Telegram if configured
	if a.cfg.TelegramToken != "" && a.cfg.TelegramChatID != "" {
		err := sendTelegramMessage(&a.cfg, summary)
		if err != nil {
			fmt.Printf("Failed to send Telegram message: %v\n", err)
		}
	}
	return nil
}

func buildForecastTable(days []weather.ForecastDay) string {
	var b strings.Builder
	b.WriteString("Date        | Wind Max | Gust Max | Dir\n")
	b.WriteString("------------+----------+---------+------\n")
	for _, day := range days {
		b.WriteString(fmt.Sprintf("%s | %8.1f | %7.1f | %s\n",
			day.Date.Format("2006-01-02"),
			day.WindSpeedMax,
			day.WindGustMax,
			degToCompass(day.WindDirMean),
		))
	}
	return b.String()
}

func buildPrompt(location string, days []weather.ForecastDay, table string) string {
	return fmt.Sprintf(`Summarize the next 15 days wind forecast for %s in a compact way:
- What is the main (predominant) wind direction?
- On which dates does the wind direction change, and what is the new direction?
- List all periods with easterly winds (E, ENE, ESE, or SE) and their dates.
- Output should be concise, suitable for a quick daily aviation risk check.

Tabular data:
%s

`, location, table)
}

// degToCompass converts degrees to compass direction (e.g., N, NE, E, etc.)
func degToCompass(deg float64) string {
	dirs := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	idx := int((deg/22.5)+0.5) % 16
	return dirs[idx]
}

func fallbackLocation(name string) string {
	if strings.TrimSpace(name) == "" {
		return "the target location"
	}
	return name
}

// TelegramMessage is the payload for Telegram API
type TelegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func sendTelegramMessage(config *Config, message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", config.TelegramToken)

	msg := TelegramMessage{
		ChatID:    config.TelegramChatID,
		Text:      message,
		ParseMode: "Markdown",
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal telegram message: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create telegram request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}
	errClose := resp.Body.Close()
	if errClose != nil {
		fmt.Printf("warning: close telegram response body: %v\n", errClose)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	return nil
}
