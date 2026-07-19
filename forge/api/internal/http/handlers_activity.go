package http

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gamepanel/forge/internal/services/activity"

	"github.com/gofiber/fiber/v2"
)

func registerActivityRoutes(protected fiber.Router, cfg Config) {
	// /account/activity is reserved for the current user's audit log, registered
	// by registerAuthRoutes. Service activity is administrative and global.
	admin := protected.Group("/admin", requireRole("admin"))
	admin.Get("/activity", func(c *fiber.Ctx) error {
		if err := requireActivityService(cfg); err != nil {
			return err
		}
		return handleQueryActivity(c, cfg)
	})
	admin.Get("/activity/stats", func(c *fiber.Ctx) error {
		if err := requireActivityService(cfg); err != nil {
			return err
		}
		return handleActivityStats(c, cfg)
	})
	admin.Get("/activity/export", func(c *fiber.Ctx) error {
		if err := requireActivityService(cfg); err != nil {
			return err
		}
		return handleExportActivity(c, cfg)
	})
}

func handleQueryActivity(c *fiber.Ctx, cfg Config) error {
	svc := cfg.ActivityService
	if svc == nil {
		return c.JSON(fiber.Map{"events": []any{}, "total": 0})
	}

	filter := activityFilterFromRequest(c)

	ctx, cancel := requestContext()
	defer cancel()

	events, err := svc.Query(ctx, filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	total, err := svc.Count(ctx, filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"events": events,
		"total":  total,
	})
}

func handleActivityStats(c *fiber.Ctx, cfg Config) error {
	svc := cfg.ActivityService
	if svc == nil {
		return c.JSON(fiber.Map{})
	}

	ctx, cancel := requestContext()
	defer cancel()

	stats, err := svc.Stats(ctx)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(stats)
}

func handleExportActivity(c *fiber.Ctx, cfg Config) error {
	svc := cfg.ActivityService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "activity service is not available")
	}

	format := strings.ToLower(c.Query("format", "json"))

	// Export uses the same filter contract as the canonical admin activity query,
	// while requesting the largest result set supported by the service.
	filter := activityFilterFromRequest(c)
	filter.Limit = 200
	filter.Offset = 0

	ctx, cancel := requestContext()
	defer cancel()

	events, err := svc.Query(ctx, filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	switch format {
	case "csv":
		c.Set("Content-Type", "text/csv")
		c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=activity_export_%s.csv", time.Now().Format("20060102_150405")))
		c.Response().SetBodyStreamWriter(func(bw *bufio.Writer) {
			w := csv.NewWriter(bw)
			w.Write([]string{"id", "event", "description", "actorId", "actorEmail", "actorType", "ip", "subjectType", "subjectId", "subjectName", "level", "source", "timestamp"})
			for _, e := range events {
				actorID := ""
				if e.ActorID != nil {
					actorID = *e.ActorID
				}
				actorEmail := ""
				if e.ActorEmail != nil {
					actorEmail = *e.ActorEmail
				}
				ip := ""
				if e.IP != nil {
					ip = *e.IP
				}
				subjectID := ""
				if e.SubjectID != nil {
					subjectID = *e.SubjectID
				}
				subjectName := ""
				if e.SubjectName != nil {
					subjectName = *e.SubjectName
				}
				subjectType := ""
				if e.SubjectType != nil {
					subjectType = *e.SubjectType
				}
				w.Write(csvSafeRecord([]string{
					e.ID, e.Event, e.Description,
					actorID, actorEmail, e.ActorType,
					ip, subjectType, subjectID, subjectName,
					string(e.Level), e.Source,
					e.Timestamp.Format(time.RFC3339),
				}))
			}
			w.Flush()
			bw.Flush()
		})
		return nil
	default:
		c.Set("Content-Type", "application/json")
		c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=activity_export_%s.json", time.Now().Format("20060102_150405")))
		data, err := json.Marshal(events)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Send(data)
	}
}

func requireActivityService(cfg Config) error {
	if cfg.ActivityService == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "activity service is not available")
	}
	return nil
}

func activityFilterFromRequest(c *fiber.Ctx) activity.ActivityFilter {
	filter := activity.ActivityFilter{}

	if v := c.Query("actorId"); v != "" {
		filter.ActorID = &v
	}
	if v := c.Query("subjectType"); v != "" {
		filter.SubjectType = &v
	}
	if v := c.Query("subjectId"); v != "" {
		filter.SubjectID = &v
	}
	if v := c.Query("event"); v != "" {
		filter.Event = &v
	}
	if v := c.Query("level"); v != "" {
		level := activity.Level(v)
		filter.Level = &level
	}
	if v := c.Query("source"); v != "" {
		filter.Source = &v
	}
	if v := c.Query("from"); v != "" {
		if timestamp, err := time.Parse(time.RFC3339, v); err == nil {
			filter.From = &timestamp
		}
	}
	if v := c.Query("to"); v != "" {
		if timestamp, err := time.Parse(time.RFC3339, v); err == nil {
			filter.To = &timestamp
		}
	}
	if v := c.Query("limit"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil {
			filter.Limit = limit
		}
	}
	if v := c.Query("offset"); v != "" {
		if offset, err := strconv.Atoi(v); err == nil {
			filter.Offset = offset
		}
	}

	return filter
}

func csvSafeRecord(record []string) []string {
	for i, value := range record {
		if value != "" && strings.ContainsRune("=+-@", rune(value[0])) {
			record[i] = "'" + value
		}
	}
	return record
}
