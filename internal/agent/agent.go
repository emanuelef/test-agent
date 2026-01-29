package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/emanuelefumagalli/test-agent/internal/ollama"
	"github.com/emanuelefumagalli/test-agent/internal/weather"
)

// Config wires together the dependencies and runtime options for the agent.
type Config struct {
	LocationName string
	ForecastDays int
	Weather      weather.Forecaster
	Ollama       *ollama.Client
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
	summary, err := a.cfg.Ollama.Generate(ctx, prompt)
	if err != nil {
		return fmt.Errorf("ollama summary: %w", err)
	}

	fmt.Println("\nOllama summary:")
	fmt.Println(summary)
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
	var timeline strings.Builder
	var windDirs []float64
	for _, day := range days {
		timeline.WriteString(fmt.Sprintf("%s - max wind %.1f km/h, gusts %.1f km/h, dir %s\n",
			day.Date.Format("Mon 02 Jan"), day.WindSpeedMax, day.WindGustMax, degToCompass(day.WindDirMean)))
		windDirs = append(windDirs, day.WindDirMean)
	}

	predominant := predominantDirection(windDirs)

	return fmt.Sprintf(`You are a weather risk assistant. Analyze the upcoming wind pattern for %s based on the forecast below.
- Highlight the windiest periods and any noticeable gust spikes.
- Recommend precautions an airport operations team should consider.
- The predominant wind direction is %s.

Tabular data:
%s

Chronological summary:
%s
`, location, predominant, table, timeline.String())
}

// degToCompass converts degrees to compass direction (e.g., N, NE, E, etc.)
func degToCompass(deg float64) string {
	dirs := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	idx := int((deg/22.5)+0.5) % 16
	return dirs[idx]
}

// predominantDirection returns the most common compass direction in the slice
func predominantDirection(degs []float64) string {
	if len(degs) == 0 {
		return "unknown"
	}
	bins := make(map[string]int)
	maxCount := 0
	var predominant string
	for _, d := range degs {
		dir := degToCompass(d)
		bins[dir]++
		if bins[dir] > maxCount {
			maxCount = bins[dir]
			predominant = dir
		}
	}
	return predominant
}

func fallbackLocation(name string) string {
	if strings.TrimSpace(name) == "" {
		return "the target location"
	}
	return name
}
