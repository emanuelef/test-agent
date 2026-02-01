package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/emanuelefumagalli/test-agent/internal/ollama"
	"github.com/emanuelefumagalli/test-agent/internal/weather"
)

// Config wires together the dependencies and runtime options for the agent.
type Config struct {
	// Wind check (Heathrow)
	WindLocation string
	WindDays     int
	WindWeather  *weather.OpenMeteoClient
	WindHour     int // UTC

	// Rain check (Twickenham)
	RainLocation string
	RainDays     int
	RainWeather  *weather.OpenMeteoClient
	RainHour     int // London time

	Ollama         *ollama.Client
	TelegramToken  string
	TelegramChatID string
}

// Agent coordinates weather checks.
type Agent struct {
	cfg Config
}

// New returns a fully constructed Agent.
func New(cfg Config) *Agent {
	if cfg.WindDays <= 0 {
		cfg.WindDays = 15
	}
	if cfg.RainDays <= 0 {
		cfg.RainDays = 7
	}
	if cfg.WindHour == 0 {
		cfg.WindHour = 10
	}
	if cfg.RainHour == 0 {
		cfg.RainHour = 8
	}
	return &Agent{cfg: cfg}
}

// Run starts both wind and rain checks concurrently.
func (a *Agent) Run(ctx context.Context) error {
	errCh := make(chan error, 2)

	// Wind check goroutine (10am UTC)
	go func() {
		errCh <- a.runWindCheck(ctx)
	}()

	// Rain check goroutine (8am London)
	go func() {
		errCh <- a.runRainCheck(ctx)
	}()

	// Wait for either to fail or context cancel
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Agent) runWindCheck(ctx context.Context) error {
	// Run immediately on startup
	fmt.Println("🛫 Wind check: running now...")
	a.doWindCheck(ctx)

	for {
		// Then sleep until next run (10am UTC)
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), a.cfg.WindHour, 0, 0, 0, time.UTC)
		if !now.Before(next) {
			next = next.Add(24 * time.Hour)
		}
		fmt.Printf("🛫 Wind check: next run at %s\n", next.Format("Mon 02 Jan 15:04 UTC"))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Until(next)):
		}

		a.doWindCheck(ctx)
	}
}

func (a *Agent) doWindCheck(ctx context.Context) {
	forecast, err := a.cfg.WindWeather.Fetch(ctx, a.cfg.WindDays)
	if err != nil {
		fmt.Printf("fetch wind forecast: %v\n", err)
		return
	}

	report := buildForecastTable(forecast)
	analysis := buildEasterlyAnalysis(forecast)

	fmt.Printf("\n🛫 %d-day %s wind forecast:\n%s%s\n", len(forecast), a.cfg.WindLocation, report, analysis)

	prompt := fmt.Sprintf(`%s wind forecast. Easterly wind = planes overhead (✈️).

%s
%s
Summarize briefly: how many easterly days and when does wind change direction?`, a.cfg.WindLocation, analysis, report)

	summary, err := a.cfg.Ollama.Generate(ctx, prompt)
	msg := analysis + "\n" + formatTelegramTable(report)
	if err == nil {
		msg += "\n" + summary
	}
	a.sendTelegram(msg)
}

func (a *Agent) runRainCheck(ctx context.Context) error {
	london, _ := time.LoadLocation("Europe/London")

	// Run immediately on startup
	fmt.Println("🌧️ Rain check: running now...")
	a.doRainCheck(ctx)

	for {
		// Then sleep until next run (8am London)
		now := time.Now().In(london)
		next := time.Date(now.Year(), now.Month(), now.Day(), a.cfg.RainHour, 0, 0, 0, london)
		if !now.Before(next) {
			next = next.Add(24 * time.Hour)
		}
		fmt.Printf("🌧️ Rain check: next run at %s\n", next.Format("Mon 02 Jan 15:04 MST"))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Until(next)):
		}

		a.doRainCheck(ctx)
	}
}

func (a *Agent) doRainCheck(ctx context.Context) {
	forecast, err := a.cfg.RainWeather.FetchRain(ctx, a.cfg.RainDays)
	if err != nil {
		fmt.Printf("fetch rain forecast: %v\n", err)
		return
	}

	report := buildRainTable(forecast)
	schoolRun := analyzeSchoolRun(forecast)

	fmt.Printf("\n🌧️ %d-day %s rain forecast:\n%s%s\n", len(forecast), a.cfg.RainLocation, report, schoolRun)

	prompt := fmt.Sprintf(`%s 7-day rain forecast. School run is 8-9am.

TODAY: %s

%s
Brief friendly summary: will it rain during school run today? Any rainy days this week?`, a.cfg.RainLocation, schoolRun, report)

	summary, err := a.cfg.Ollama.Generate(ctx, prompt)
	msg := schoolRun + "\n" + formatTelegramTable(report)
	if err == nil {
		msg += "\n" + summary
	}
	a.sendTelegram(msg)
}

func (a *Agent) sendTelegram(msg string) {
	if a.cfg.TelegramToken == "" || a.cfg.TelegramChatID == "" {
		return
	}
	if err := sendTelegramMessage(a.cfg.TelegramToken, a.cfg.TelegramChatID, msg); err != nil {
		fmt.Printf("Telegram failed: %v\n", err)
	}
}

func buildRainTable(days []weather.RainForecast) string {
	var b strings.Builder
	b.WriteString("Date       | Rain% | Drop | Pick\n")
	b.WriteString("-----------+-------+------+------\n")
	for _, day := range days {
		weekday := day.Date.Weekday()

		// Skip weekends
		if weekday == time.Saturday || weekday == time.Sunday {
			b.WriteString(fmt.Sprintf("%s | %4d%% |  --  |  -- \n",
				day.Date.Format("Mon 02 Jan"),
				day.PrecipProb,
			))
			continue
		}

		dropProb := getHourProb(day, 8, 9)
		pickProb := getPickupProb(day, weekday)

		dropIcon := "   "
		if dropProb >= 30 {
			dropIcon = " ☔"
		}
		pickIcon := "   "
		if pickProb >= 30 {
			pickIcon = " ☔"
		}

		b.WriteString(fmt.Sprintf("%s | %4d%% |%s |%s\n",
			day.Date.Format("Mon 02 Jan"),
			day.PrecipProb,
			dropIcon,
			pickIcon,
		))
	}
	return b.String()
}

func getHourProb(day weather.RainForecast, startHour, endHour int) int {
	if len(day.MorningRainProb) == 0 {
		return day.PrecipProb
	}
	// MorningRainProb covers hours 6,7,8,9,10 (indices 0,1,2,3,4)
	maxProb := 0
	for i := startHour - 6; i <= endHour-6 && i < len(day.MorningRainProb); i++ {
		if i >= 0 && day.MorningRainProb[i] > maxProb {
			maxProb = day.MorningRainProb[i]
		}
	}
	if maxProb == 0 {
		return day.PrecipProb
	}
	return maxProb
}

func getPickupProb(day weather.RainForecast, weekday time.Weekday) int {
	// AfternoonProb covers hours 15,16,17,18 (indices 0,1,2,3)
	if len(day.AfternoonProb) == 0 {
		return day.PrecipProb
	}

	var maxProb int
	if weekday == time.Wednesday {
		// Wednesday: 15:15-16:00 (indices 0,1)
		for i := 0; i <= 1 && i < len(day.AfternoonProb); i++ {
			if day.AfternoonProb[i] > maxProb {
				maxProb = day.AfternoonProb[i]
			}
		}
	} else {
		// Other days: 17:00-18:00 (indices 2,3)
		for i := 2; i <= 3 && i < len(day.AfternoonProb); i++ {
			if day.AfternoonProb[i] > maxProb {
				maxProb = day.AfternoonProb[i]
			}
		}
	}

	if maxProb == 0 {
		return day.PrecipProb
	}
	return maxProb
}

func analyzeSchoolRun(days []weather.RainForecast) string {
	if len(days) == 0 {
		return "No forecast data"
	}
	today := days[0]
	weekday := today.Date.Weekday()

	// Weekend - no school
	if weekday == time.Saturday || weekday == time.Sunday {
		return "📅 Weekend - no school!"
	}

	dropProb := getHourProb(today, 8, 9)
	pickProb := getPickupProb(today, weekday)

	// Pickup time info
	pickTime := "17-18"
	if weekday == time.Wednesday {
		pickTime = "15:15-16"
	}

	var result strings.Builder

	// Drop-off analysis
	if dropProb >= 70 {
		result.WriteString(fmt.Sprintf("☔ DROP-OFF (8-9am): %d%% - Umbrella!\n", dropProb))
	} else if dropProb >= 30 {
		result.WriteString(fmt.Sprintf("🌦️ DROP-OFF (8-9am): %d%% - Maybe umbrella\n", dropProb))
	} else {
		result.WriteString(fmt.Sprintf("☀️ DROP-OFF (8-9am): %d%%\n", dropProb))
	}

	// Pickup analysis
	if pickProb >= 70 {
		result.WriteString(fmt.Sprintf("☔ PICKUP (%s): %d%% - Umbrella!", pickTime, pickProb))
	} else if pickProb >= 30 {
		result.WriteString(fmt.Sprintf("🌦️ PICKUP (%s): %d%% - Maybe umbrella", pickTime, pickProb))
	} else {
		result.WriteString(fmt.Sprintf("☀️ PICKUP (%s): %d%%", pickTime, pickProb))
	}

	return result.String()
}

// formatTelegramTable wraps the table in Markdown code block for Telegram
func formatTelegramTable(table string) string {
	return "```\n" + table + "```"
}

func buildForecastTable(days []weather.ForecastDay) string {
	var b strings.Builder
	b.WriteString("Date       | Wind | Dir | East\n")
	b.WriteString("-----------+------+-----+-----\n")
	for _, day := range days {
		eastMarker := "   "
		if isEasterly(day.WindDirMean) {
			eastMarker = " ✈️"
		}
		b.WriteString(fmt.Sprintf("%s | %4.0f | %-3s |%s\n",
			day.Date.Format("Mon 02 Jan"),
			day.WindSpeedMax,
			degToCompass(day.WindDirMean),
			eastMarker,
		))
	}
	return b.String()
}

// degToCompass converts degrees to E or W (what matters for flight paths)
func degToCompass(deg float64) string {
	deg = float64(int(deg+360) % 360)
	// East: 0-180, West: 180-360
	if deg > 0 && deg < 180 {
		return "E"
	}
	return "W"
}

// isEasterly returns true if wind is from the east
func isEasterly(deg float64) bool {
	deg = float64(int(deg+360) % 360)
	return deg > 0 && deg < 180
}

// countEasterlyDays counts how many days have easterly winds
func countEasterlyDays(days []weather.ForecastDay) int {
	count := 0
	for _, d := range days {
		if isEasterly(d.WindDirMean) {
			count++
		}
	}
	return count
}

// buildEasterlyAnalysis creates a simple summary with dominant direction
func buildEasterlyAnalysis(days []weather.ForecastDay) string {
	eastCount := countEasterlyDays(days)
	westCount := len(days) - eastCount

	var dominant string
	if eastCount > westCount {
		dominant = "E ✈️"
	} else if westCount > eastCount {
		dominant = "W"
	} else {
		dominant = "Mixed"
	}

	return fmt.Sprintf("Dominant: %s | East: %d days | West: %d days\n", dominant, eastCount, westCount)
}

// TelegramMessage is the payload for Telegram API
type TelegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

func sendTelegramMessage(token, chatID, message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)

	msg := TelegramMessage{
		ChatID:    chatID,
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
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			fmt.Printf("warning: close telegram response body: %v\n", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
