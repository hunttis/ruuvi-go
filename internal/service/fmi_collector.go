package service

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	fmiBaseURL  = "https://opendata.fmi.fi/wfs"
	bsWfsNS     = "http://xml.fmi.fi/schema/wfs/2.0"
	fmiCacheTTL = 30 * time.Minute
	fmiHorizon  = 6 * time.Hour
)

var weatherSymbols = map[int]string{
	1:  "Clear",
	2:  "Partly cloudy",
	3:  "Cloudy",
	21: "Light showers",
	22: "Showers",
	23: "Heavy showers",
	31: "Light rain",
	32: "Rain",
	33: "Heavy rain",
	51: "Light snow",
	52: "Snow",
	53: "Heavy snow",
	61: "Thunder",
	71: "Light sleet sh.",
	72: "Sleet showers",
	73: "Heavy sleet sh.",
	81: "Light sleet",
	82: "Sleet",
	83: "Heavy sleet",
}

// weatherIcons maps FMI WeatherSymbol3 codes to TRMNL hosted SVG icon slugs.
// Only 5 icons exist per layout dir: sunny, cloudy-gusts, showers, snow, windy.
// Icons served from https://trmnl.com/images/plugins/weather/[layout]/[slug].svg
var weatherIcons = map[int]string{
	1:  "sunny",
	2:  "cloudy-gusts",
	3:  "cloudy-gusts",
	21: "showers",
	22: "showers",
	23: "showers",
	31: "showers",
	32: "showers",
	33: "showers",
	51: "snow",
	52: "snow",
	53: "snow",
	61: "showers", // no thunder icon available
	71: "snow",    // sleet → snow is closest
	72: "snow",
	73: "snow",
	81: "snow",
	82: "snow",
	83: "snow",
}

// ForecastHour holds weather data for one forecast step.
type ForecastHour struct {
	Time          string  `json:"time"`          // HH:MM in Helsinki timezone
	Temperature   float64 `json:"temperature"`   // °C
	FeelsLike     float64 `json:"feelsLike"`     // °C
	SymbolText    string  `json:"symbolText"`    // human-readable condition
	SymbolIcon    string  `json:"symbolIcon"`    // TRMNL SVG icon slug
	Precipitation float64 `json:"precipitation"` // mm/h
}

// WeatherData holds current conditions plus next hourly forecasts.
type WeatherData struct {
	Current  ForecastHour   `json:"current"`
	Forecast []ForecastHour `json:"forecast"` // next 5 hours
}

// FmiCollector fetches and caches FMI open data forecasts.
type FmiCollector struct {
	mu       sync.Mutex
	cached   *WeatherData
	cacheAt  time.Time
	lastErr  error
	place    string
	client   *http.Client
	location *time.Location
}

// NewFmiCollector creates a collector for the given place name (e.g. "Helsinki").
func NewFmiCollector(place string) *FmiCollector {
	loc, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		log.Printf("fmi: cannot load Europe/Helsinki timezone (%v); using UTC+2", err)
		loc = time.FixedZone("EET", 2*3600)
	}
	return &FmiCollector{
		place:    place,
		client:   &http.Client{Timeout: 15 * time.Second},
		location: loc,
	}
}

// LastFetchedAt returns the time the cache was last successfully populated (zero if never).
func (f *FmiCollector) LastFetchedAt() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cacheAt
}

// LastError returns the most recent fetch error, or nil if the last fetch succeeded.
func (f *FmiCollector) LastError() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastErr
}

// Status returns a short human-readable status string for display in the UI.
func (f *FmiCollector) Status() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastErr != nil {
		return "FMI error: " + f.lastErr.Error()
	}
	if f.cacheAt.IsZero() {
		return "FMI: fetching…"
	}
	return "FMI: " + timeAgoSimple(f.cacheAt)
}

func timeAgoSimple(t time.Time) string {
	d := time.Since(t).Round(time.Second)
	switch {
	case d < 5*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

// Get returns weather data, using a 30-minute cache.
// Falls back to stale cached data if the API is unreachable.
func (f *FmiCollector) Get() (*WeatherData, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.cached != nil && time.Since(f.cacheAt) < fmiCacheTTL {
		return f.cached, nil
	}

	data, err := f.fetch()
	if err != nil {
		f.lastErr = err
		if f.cached != nil {
			log.Printf("fmi: fetch failed (%v); returning stale cache", err)
			return f.cached, nil
		}
		return nil, err
	}

	f.lastErr = nil
	f.cached = data
	f.cacheAt = time.Now()
	return f.cached, nil
}

func (f *FmiCollector) fetch() (*WeatherData, error) {
	now := time.Now().UTC()
	end := now.Add(fmiHorizon)

	// Build URL manually to avoid url.Values percent-encoding the colons in
	// the storedquery_id (fmi::forecast::...) which the FMI server rejects.
	reqURL := fmt.Sprintf(
		"%s?service=WFS&version=2.0.0&request=getFeature"+
			"&storedquery_id=fmi::forecast::harmonie::surface::point::simple"+
			"&place=%s&parameters=Temperature,WeatherSymbol3,Precipitation1h"+
			"&timestep=60&starttime=%s&endtime=%s",
		fmiBaseURL,
		url.QueryEscape(f.place),
		now.Format(time.RFC3339),
		end.Format(time.RFC3339),
	)
	log.Printf("fmi: fetching %s", reqURL)

	resp, err := f.client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Log the first 500 bytes of the error body to help diagnose API issues.
		snippet := string(body)
		if len(snippet) > 500 {
			snippet = snippet[:500] + "…"
		}
		log.Printf("fmi: server returned %d: %s", resp.StatusCode, snippet)
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	elements, err := parseBsWfsElements(body)
	if err != nil {
		return nil, fmt.Errorf("parse xml: %w", err)
	}

	if len(elements) == 0 {
		// Log the body so we can see what the API actually returned.
		snippet := string(body)
		if len(snippet) > 500 {
			snippet = snippet[:500] + "…"
		}
		log.Printf("fmi: parsed 0 elements from response: %s", snippet)
		return nil, fmt.Errorf("no BsWfsElements in response")
	}

	return f.buildWeatherData(elements)
}

type rawWfsElement struct {
	time      string
	paramName string
	value     string
}

// parseBsWfsElements decodes BsWfsElement nodes from a WFS XML response.
func parseBsWfsElements(data []byte) ([]rawWfsElement, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var elements []rawWfsElement
	var cur *rawWfsElement
	var field string

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space == bsWfsNS && t.Name.Local == "BsWfsElement" {
				cur = &rawWfsElement{}
			} else if cur != nil && t.Name.Space == bsWfsNS {
				field = t.Name.Local
			}
		case xml.CharData:
			if cur != nil && field != "" {
				s := strings.TrimSpace(string(t))
				switch field {
				case "Time":
					cur.time = s
				case "ParameterName":
					cur.paramName = s
				case "ParameterValue":
					cur.value = s
				}
			}
		case xml.EndElement:
			if t.Name.Space == bsWfsNS {
				if t.Name.Local == "BsWfsElement" && cur != nil {
					elements = append(elements, *cur)
					cur = nil
				}
				field = ""
			}
		}
	}
	return elements, nil
}

type hourParams struct {
	temperature   float64
	feelsLike     float64
	weatherSymbol float64
	precipitation float64
}

func (f *FmiCollector) buildWeatherData(elements []rawWfsElement) (*WeatherData, error) {
	byTime := make(map[string]*hourParams)
	var orderedTimes []string

	for _, el := range elements {
		if _, exists := byTime[el.time]; !exists {
			byTime[el.time] = &hourParams{}
			orderedTimes = append(orderedTimes, el.time)
		}
		val, err := strconv.ParseFloat(el.value, 64)
		if err != nil || math.IsNaN(val) {
			continue
		}
		p := byTime[el.time]
		switch el.paramName {
		case "Temperature":
			p.temperature = val
		case "WeatherSymbol3":
			p.weatherSymbol = val
		case "Precipitation1h":
			p.precipitation = val
		}
	}

	// Deduplicate while preserving order, then sort chronologically.
	seen := make(map[string]bool)
	var times []string
	for _, t := range orderedTimes {
		if !seen[t] {
			seen[t] = true
			times = append(times, t)
		}
	}
	sort.Strings(times)

	if len(times) == 0 {
		return nil, fmt.Errorf("no forecast data in response")
	}

	toHour := func(ts string, p *hourParams) ForecastHour {
		t, _ := time.Parse(time.RFC3339, ts)
		local := t.In(f.location)
		sym := int(math.Round(p.weatherSymbol))
		text, ok := weatherSymbols[sym]
		if !ok && sym != 0 {
			text = fmt.Sprintf("(%d)", sym)
		}
		icon := weatherIcons[sym]
		if icon == "" {
			icon = "cloudy-gusts"
		}
		prec := math.Round(p.precipitation*10) / 10
		return ForecastHour{
			Time:          local.Format("15:04"),
			Temperature:   p.temperature,
			FeelsLike:     p.temperature, // HIRLAM doesn't provide FeelsLike
			SymbolText:    text,
			SymbolIcon:    icon,
			Precipitation: prec,
		}
	}

	current := toHour(times[0], byTime[times[0]])

	var forecast []ForecastHour
	for i := 1; i < len(times) && len(forecast) < 5; i++ {
		forecast = append(forecast, toHour(times[i], byTime[times[i]]))
	}

	return &WeatherData{Current: current, Forecast: forecast}, nil
}
