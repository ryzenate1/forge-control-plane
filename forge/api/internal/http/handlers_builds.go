package http

import (
	"bufio"
	"time"

	"gamepanel/forge/internal/services/build"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

type startBuildRequest struct {
	SourceID    string            `json:"sourceId" validate:"required"`
	BuilderType build.BuilderType `json:"builderType"`
	SourceDir   string            `json:"sourceDir" validate:"required"`
	ImageName   string            `json:"imageName"`
	Dockerfile  string            `json:"dockerfile"`
	BuildArgs   []string          `json:"buildArgs"`
	Labels      []string          `json:"labels"`
	Tags        []string          `json:"tags"`
	NoCache     bool              `json:"noCache"`
}

func registerBuildRoutes(v1 fiber.Router, cfg Config, buildSvc *build.Service, mutationLimiter fiber.Handler) {
	if buildSvc == nil {
		return
	}

	builds := v1.Group("/builds")

	builds.Post("/", mutationLimiter, func(c *fiber.Ctx) error {
		var req startBuildRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		if req.SourceID == "" || req.SourceDir == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "sourceId and sourceDir are required"})
		}

		builderType := req.BuilderType
		if builderType == "" {
			detected, detectErr := buildSvc.Detect(req.SourceDir)
			if detectErr != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": detectErr.Error()})
			}
			builderType = detected
		}

		logCh := make(chan build.BuildLogEntry, 256)
		record, err := buildSvc.StartBuild(c.Context(), req.SourceID, builderType, build.BuildOptions{
			SourceDir:  req.SourceDir,
			Dockerfile: req.Dockerfile,
			ImageName:  req.ImageName,
			BuildArgs:  req.BuildArgs,
			Labels:     req.Labels,
			Tags:       req.Tags,
			NoCache:    req.NoCache,
		}, logCh)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"data": record})
	})

	builds.Get("/", func(c *fiber.Ctx) error {
		sourceID := c.Query("sourceId")
		records, err := buildSvc.ListBuilds(c.Context(), sourceID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		if records == nil {
			records = []*store.BuildRecord{}
		}
		return c.JSON(fiber.Map{"data": records})
	})

	builds.Get("/:id", func(c *fiber.Ctx) error {
		record, err := buildSvc.GetBuild(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": record})
	})

	builds.Get("/:id/logs", func(c *fiber.Ctx) error {
		buildID := c.Params("id")
		record, err := buildSvc.GetBuild(c.Context(), buildID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}

		if build.IsTerminal(build.BuildStatus(record.Status)) {
			if record.BuildLog != "" {
				c.Set("Content-Type", "text/plain")
				return c.SendString(record.BuildLog)
			}
			return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": []string{}, "terminal": true})
		}

		follow := c.Query("follow", "true")
		if follow != "true" {
			return c.JSON(fiber.Map{"data": record, "terminal": false})
		}

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("Transfer-Encoding", "chunked")

		logCh, err := buildSvc.StreamLogs(c.Context(), buildID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}

		ctx := c.Context()
		c.Response().SetBodyStreamWriter(func(bw *bufio.Writer) {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case entry, ok := <-logCh:
					if !ok {
						return
					}
					_, _ = bw.Write([]byte("data: " + entry.Line + "\n\n"))
				case <-ticker.C:
					record, checkErr := buildSvc.GetBuild(c.Context(), buildID)
					if checkErr == nil && build.IsTerminal(build.BuildStatus(record.Status)) {
						_, _ = bw.Write([]byte("event: done\ndata: " + record.Status + "\n\n"))
						return
					}
					_ = bw.Flush()
				case <-ctx.Done():
					return
				}
			}
		})
		return nil
	})

	builds.Post("/:id/cancel", mutationLimiter, func(c *fiber.Ctx) error {
		if err := buildSvc.CancelBuild(c.Context(), c.Params("id")); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})
}
