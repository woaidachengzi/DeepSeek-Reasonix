package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	configpkg "reasonix/internal/config"
	"reasonix/internal/desktopbridge"
	"reasonix/internal/stats"
)

type previewUsageStatsRequest struct {
	Range  string `json:"range"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Source string `json:"source,omitempty"`
}

var errInvalidPreviewUsageRequest = errors.New("invalid Preview usage statistics request")

type previewUsageStatsResponse struct {
	ProtocolVersion int                   `json:"protocolVersion"`
	From            string                `json:"from"`
	To              string                `json:"to"`
	Tokens          int64                 `json:"tokens"`
	Requests        int                   `json:"requests"`
	Turns           int                   `json:"turns"`
	CacheHit        int64                 `json:"cacheHit"`
	CacheMiss       int64                 `json:"cacheMiss"`
	ActiveDays      int                   `json:"activeDays"`
	TopModel        string                `json:"topModel"`
	TopProvider     string                `json:"topProvider"`
	Daily           []stats.DailyTokens   `json:"daily"`
	Models          []stats.ModelUsage    `json:"models"`
	Providers       []stats.ProviderUsage `json:"providers"`
}

func previewStatsRange(req previewUsageStatsRequest, now time.Time) (time.Time, time.Time, error) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	to := today.AddDate(0, 0, 1).Add(-time.Nanosecond)
	switch req.Range {
	case "7", "14", "30", "90":
		days, _ := strconv.Atoi(req.Range)
		return today.AddDate(0, 0, -(days - 1)), to, nil
	case "custom":
		from, fromErr := time.ParseInLocation("2006-01-02", req.From, now.Location())
		end, toErr := time.ParseInLocation("2006-01-02", req.To, now.Location())
		if fromErr != nil || toErr != nil || from.Format("2006-01-02") != req.From || end.Format("2006-01-02") != req.To || end.Before(from) || end.After(today) {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: date range", errInvalidPreviewUsageRequest)
		}
		fromUTC := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
		toUTC := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
		if days := int(toUTC.Sub(fromUTC)/(24*time.Hour)) + 1; days > 3660 {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: date range exceeds ten years", errInvalidPreviewUsageRequest)
		}
		return from, end.AddDate(0, 0, 1).Add(-time.Nanosecond), nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("%w: unsupported range", errInvalidPreviewUsageRequest)
	}
}

func queryPreviewUsageStats(req previewUsageStatsRequest) (previewUsageStatsResponse, error) {
	from, to, err := previewStatsRange(req, time.Now())
	if err != nil {
		return previewUsageStatsResponse{}, err
	}
	switch req.Source {
	case "", "all", "desktop-tauri":
	default:
		return previewUsageStatsResponse{}, fmt.Errorf("%w: unsupported source", errInvalidPreviewUsageRequest)
	}
	statsDir := configpkg.StatsDir()
	if statsDir == "" {
		return previewUsageStatsResponse{}, fmt.Errorf("resolve Preview statistics directory")
	}
	flushCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	_ = stats.Flush(flushCtx, statsDir)
	cancel()
	result, err := stats.NewWriter(statsDir).Query(stats.SourceFilter{From: from, To: to, Source: req.Source})
	if err != nil {
		return previewUsageStatsResponse{}, err
	}
	return previewUsageStatsResponse{
		ProtocolVersion: desktopbridge.ProtocolVersion,
		From:            result.From, To: result.To, Tokens: result.Tokens,
		Requests: result.Requests, Turns: result.Turns,
		CacheHit: result.CacheHit, CacheMiss: result.CacheMiss,
		ActiveDays: result.ActiveDays, TopModel: result.TopModel,
		TopProvider: result.TopProvider, Daily: result.Daily,
		Models: result.Models, Providers: result.Providers,
	}, nil
}
