// media-ingest (mingest) - Media Ingestion CLI tool
// Copyright (C) 2026  Harrison Wang <https://mingest.com>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package ingest

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

func configureLogger() {
	level := parseLogLevel(os.Getenv("MINGEST_LOG_LEVEL"))
	options := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				if ts, ok := attr.Value.Any().(time.Time); ok {
					attr.Value = slog.StringValue(ts.UTC().Format(time.RFC3339))
				}
			}
			return attr
		},
	}

	var handler slog.Handler
	format := strings.ToLower(strings.TrimSpace(os.Getenv("MINGEST_LOG_FORMAT")))
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, options)
	} else {
		handler = slog.NewTextHandler(os.Stderr, options)
	}

	slog.SetDefault(slog.New(handler))
}

func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func logDebug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

func logInfo(msg string, args ...any) {
	slog.Info(msg, args...)
}

func logWarn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

func logError(msg string, args ...any) {
	slog.Error(msg, args...)
}

func logDebugf(format string, args ...any) {
	slog.Debug(fmt.Sprintf(format, args...))
}

func logInfof(format string, args ...any) {
	slog.Info(fmt.Sprintf(format, args...))
}

func logWarnf(format string, args ...any) {
	slog.Warn(fmt.Sprintf(format, args...))
}

func logErrorf(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
}
