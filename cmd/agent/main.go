package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/emanuelefumagalli/test-agent/internal/agent"
	"github.com/emanuelefumagalli/test-agent/internal/ollama"
	"github.com/emanuelefumagalli/test-agent/internal/weather"
)

const (
	heathrowLatitude  = 51.47
	heathrowLongitude = -0.4543
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	days := getForecastDays()

	ag := agent.New(agent.Config{
		LocationName: "London Heathrow",
		ForecastDays: days,
		Weather: &weather.OpenMeteoClient{
			Latitude:  heathrowLatitude,
			Longitude: heathrowLongitude,
		},
		Ollama: &ollama.Client{
			Host:  envOrDefault("OLLAMA_HOST", "http://127.0.0.1:11434"),
			Model: envOrDefault("OLLAMA_MODEL", "llama3.1"),
		},
	})

	if err := ag.Run(ctx); err != nil {
		log.Fatalf("agent run failed: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getForecastDays() int {
	raw := os.Getenv("FORECAST_DAYS")
	if raw == "" {
		return 15
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days < 1 {
		return 15
	}
	if days > 16 { // Open-Meteo caps forecast days at 16
		return 16
	}
	return days
}
