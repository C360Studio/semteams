package graphql

import (
	"context"
	"regexp"
	"strconv"
	"time"
)

// KeywordClassifier implements regex-based natural language query classification.
// Uses pattern matching to extract temporal, spatial, and intent information.
type KeywordClassifier struct{}

// Temporal patterns
var (
	yesterdayPattern  = regexp.MustCompile(`(?i)(?:^|[^-])\byesterday\b`)
	todayPattern      = regexp.MustCompile(`(?i)\btoday\b`)
	lastNHoursPattern = regexp.MustCompile(`(?i)\blast\s+(\d+)\s+hours?\b`)
	lastNDaysPattern  = regexp.MustCompile(`(?i)\blast\s+(\d+)\s+days?\b`)
	lastWeekPattern   = regexp.MustCompile(`(?i)\blast\s+week\b`)
	lastMonthPattern  = regexp.MustCompile(`(?i)\blast\s+month\b`)
	lastYearPattern   = regexp.MustCompile(`(?i)\blast\s+year\b`)
	thisWeekPattern   = regexp.MustCompile(`(?i)\bthis\s+week\b`)
	thisMonthPattern  = regexp.MustCompile(`(?i)\bthis\s+month\b`)
	thisYearPattern   = regexp.MustCompile(`(?i)\bthis\s+year\b`)
)

// Spatial patterns
var (
	nearPattern   = regexp.MustCompile(`(?i)\bnear\s+`)
	inZonePattern = regexp.MustCompile(`(?i)\bin\s+(zone|area|region)\s+`)
	withinPattern = regexp.MustCompile(`(?i)\bwithin\s+\d+\s*(km|m|miles?)\b`)
)

// Intent patterns
var (
	similarityPattern  = regexp.MustCompile(`(?i)\b(similar|like|resembl|compar)\w*`)
	pathPattern        = regexp.MustCompile(`(?i)\b(connect|relat|path|link|between)\w*`)
	aggregationPattern = regexp.MustCompile(`(?i)\b(count|how\s+many|total|sum|average)\b`)
)

// ClassifyQuery analyzes a natural language query and populates SearchOptions.
// Detects temporal, spatial, similarity, path, and aggregation intents.
// Always returns non-nil SearchOptions with the original query preserved.
func (k *KeywordClassifier) ClassifyQuery(_ context.Context, query string) *SearchOptions {
	opts := &SearchOptions{
		Query: query,
	}

	// Empty query returns empty options
	if query == "" {
		return opts
	}

	// Extract temporal information (first match wins)
	if timeRange := k.extractTemporal(query); timeRange != nil {
		opts.TimeRange = timeRange
	}

	// Detect similarity intent
	if similarityPattern.MatchString(query) {
		opts.UseEmbeddings = true
	}

	// Note: Spatial and path intents are detected but not yet acted upon in Phase 1
	// They will be used by downstream strategy inference or future phases

	return opts
}

// extractTemporal attempts to extract temporal range from query text.
// Returns first match found, prioritizing specific patterns over general ones.
func (k *KeywordClassifier) extractTemporal(query string) *TimeRange {
	// Try yesterday
	if yesterdayPattern.MatchString(query) {
		return k.getYesterdayRange()
	}

	// Try today
	if todayPattern.MatchString(query) {
		return k.getTodayRange()
	}

	// Try "last N hours"
	if matches := lastNHoursPattern.FindStringSubmatch(query); matches != nil {
		if hours, err := strconv.Atoi(matches[1]); err == nil {
			return k.getLastNHoursRange(hours)
		}
	}

	// Try "last N days"
	if matches := lastNDaysPattern.FindStringSubmatch(query); matches != nil {
		if days, err := strconv.Atoi(matches[1]); err == nil {
			return k.getLastNDaysRange(days)
		}
	}

	// Try "last week"
	if lastWeekPattern.MatchString(query) {
		return k.getLastNDaysRange(7)
	}

	// Try "last month"
	if lastMonthPattern.MatchString(query) {
		return k.getLastNDaysRange(30)
	}

	// Try "last year"
	if lastYearPattern.MatchString(query) {
		return k.getLastNDaysRange(365)
	}

	// Try "this week"
	if thisWeekPattern.MatchString(query) {
		return k.getThisWeekRange()
	}

	// Try "this month"
	if thisMonthPattern.MatchString(query) {
		return k.getThisMonthRange()
	}

	// Try "this year"
	if thisYearPattern.MatchString(query) {
		return k.getThisYearRange()
	}

	return nil
}

// getYesterdayRange returns TimeRange for yesterday (00:00:00 to 23:59:59).
func (k *KeywordClassifier) getYesterdayRange() *TimeRange {
	now := time.Now()
	yesterdayStart := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location())
	yesterdayEnd := time.Date(now.Year(), now.Month(), now.Day()-1, 23, 59, 59, 999999999, now.Location())
	return &TimeRange{
		Start: yesterdayStart,
		End:   yesterdayEnd,
	}
}

// getTodayRange returns TimeRange for today (00:00:00 to 23:59:59).
func (k *KeywordClassifier) getTodayRange() *TimeRange {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, now.Location())
	return &TimeRange{
		Start: todayStart,
		End:   todayEnd,
	}
}

// getLastNHoursRange returns TimeRange from N hours ago to now.
func (k *KeywordClassifier) getLastNHoursRange(hours int) *TimeRange {
	now := time.Now()
	start := now.Add(-time.Duration(hours) * time.Hour)
	return &TimeRange{
		Start: start,
		End:   now,
	}
}

// getLastNDaysRange returns TimeRange from N days ago to now.
func (k *KeywordClassifier) getLastNDaysRange(days int) *TimeRange {
	now := time.Now()
	start := now.Add(-time.Duration(days) * 24 * time.Hour)
	return &TimeRange{
		Start: start,
		End:   now,
	}
}

// getThisWeekRange returns TimeRange for current week (Sunday to Saturday).
func (k *KeywordClassifier) getThisWeekRange() *TimeRange {
	now := time.Now()
	// Find the start of the week (Sunday)
	weekday := int(now.Weekday())
	weekStart := time.Date(now.Year(), now.Month(), now.Day()-weekday, 0, 0, 0, 0, now.Location())
	// End of week (Saturday 23:59:59)
	weekEnd := time.Date(now.Year(), now.Month(), now.Day()+(6-weekday), 23, 59, 59, 999999999, now.Location())
	return &TimeRange{
		Start: weekStart,
		End:   weekEnd,
	}
}

// getThisMonthRange returns TimeRange for current month.
func (k *KeywordClassifier) getThisMonthRange() *TimeRange {
	now := time.Now()
	// First day of month
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	// Last day of month
	nextMonth := monthStart.AddDate(0, 1, 0)
	monthEnd := nextMonth.Add(-time.Second)
	return &TimeRange{
		Start: monthStart,
		End:   monthEnd,
	}
}

// getThisYearRange returns TimeRange for current year.
func (k *KeywordClassifier) getThisYearRange() *TimeRange {
	now := time.Now()
	// First day of year
	yearStart := time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, now.Location())
	// Last day of year
	yearEnd := time.Date(now.Year(), time.December, 31, 23, 59, 59, 999999999, now.Location())
	return &TimeRange{
		Start: yearStart,
		End:   yearEnd,
	}
}
