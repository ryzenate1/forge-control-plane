package http

import (
	"os"
	"strings"

	"gamepanel/forge/internal/cloud"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerCloudRoutes(protected fiber.Router, cfg Config, svc *cloud.Manager, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	cl := protected.Group("/admin/cloud", adminIPAccess)

	cl.Get("/providers", requireRole("admin"), requireAdminScope("cloud.read"), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"data": svc.ListProviders()})
	})

	cl.Get("/links", requireRole("admin"), requireAdminScope("cloud.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		links, err := cfg.Store.ListCloudNodeLinks(c.Context())
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "list cloud-node links")
		}
		return c.JSON(fiber.Map{"data": links})
	})

	cl.Get("/providers/:provider/regions", requireRole("admin"), requireAdminScope("cloud.read"), func(c *fiber.Ctx) error {
		provider, err := svc.GetProvider(cloud.ProviderKind(c.Params("provider")))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		regions, err := provider.ListRegions(c.Context())
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "cloud provider could not list regions: "+err.Error())
		}
		return c.JSON(fiber.Map{"data": regions})
	})

	cl.Get("/providers/:provider/types", requireRole("admin"), requireAdminScope("cloud.read"), func(c *fiber.Ctx) error {
		provider, err := svc.GetProvider(cloud.ProviderKind(c.Params("provider")))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		types, err := provider.ListInstanceTypes(c.Context(), c.Query("region"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "cloud provider could not list instance types: "+err.Error())
		}
		return c.JSON(fiber.Map{"data": types})
	})

	cl.Get("/instances", requireRole("admin"), requireAdminScope("cloud.read"), func(c *fiber.Ctx) error {
		provider, err := svc.GetProvider(cloud.ProviderKind(c.Query("provider")))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		instances, err := provider.ListInstances(c.Context())
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "cloud provider could not list instances: "+err.Error())
		}
		return c.JSON(fiber.Map{"data": instances})
	})

	cl.Post("/provision", mutationLimiter, requireRole("admin"), requireAdminScope("cloud.write"), func(c *fiber.Ctx) error {
		var req struct {
			Provider cloud.ProviderKind          `json:"provider"`
			Request  cloud.CreateInstanceRequest `json:"request"`
			NodeID   string                      `json:"nodeId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(string(req.Provider)) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "provider is required")
		}
		if err := req.Request.Validate(); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if strings.TrimSpace(req.NodeID) != "" {
			if cfg.Store == nil {
				return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
			}
			if _, err := cfg.Store.GetNode(c.Context(), req.NodeID); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "node not found")
			}
			credential, err := cfg.Store.GetNodeDaemonCredential(c.Context(), req.NodeID)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "load node bootstrap credential")
			}
			panelAPIURL := strings.TrimSpace(os.Getenv("BEACON_PANEL_API_URL"))
			image := strings.TrimSpace(req.Request.BeaconImage)
			if image == "" {
				image = strings.TrimSpace(os.Getenv("AWS_BEACON_IMAGE"))
			}
			userData, err := cloud.BeaconCloudInit(req.NodeID, credential, panelAPIURL, image, cloud.BeaconBackupConfig{
				Adapter:      strings.TrimSpace(os.Getenv("BACKUP_ADAPTER")),
				Bucket:       strings.TrimSpace(os.Getenv("S3_BUCKET")),
				Region:       strings.TrimSpace(os.Getenv("S3_REGION")),
				Endpoint:     strings.TrimSpace(os.Getenv("S3_ENDPOINT")),
				Prefix:       strings.TrimSpace(os.Getenv("S3_PREFIX")),
				UsePathStyle: strings.TrimSpace(os.Getenv("S3_USE_PATH_STYLE")),
			})
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, err.Error())
			}
			req.Request.UserData = userData
			if req.Request.SubnetID == "" {
				req.Request.SubnetID = strings.TrimSpace(os.Getenv("AWS_SUBNET_ID"))
			}
			if len(req.Request.SecurityGroupIDs) == 0 {
				req.Request.SecurityGroupIDs = splitCSV(os.Getenv("AWS_SECURITY_GROUP_IDS"))
			}
			if req.Request.IAMInstanceProfile == "" {
				req.Request.IAMInstanceProfile = strings.TrimSpace(os.Getenv("AWS_IAM_INSTANCE_PROFILE"))
			}
		}

		instance, err := svc.ProvisionNode(c.Context(), req.Provider, req.Request)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "cloud provider could not provision instance: "+err.Error())
		}

		var link *store.CloudNodeLink
		if strings.TrimSpace(req.NodeID) != "" {
			nodeAddress := strings.TrimSpace(instance.PrivateIP)
			if nodeAddress == "" {
				nodeAddress = strings.TrimSpace(instance.PublicIP)
			}
			if err := cfg.Store.SetNodeCloudEndpoint(c.Context(), req.NodeID, nodeAddress, 9090); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "instance was provisioned but the node endpoint could not be updated; instance ID: "+instance.ID)
			}
			link = &store.CloudNodeLink{Provider: string(req.Provider), InstanceID: instance.ID, NodeID: req.NodeID}
			if err := cfg.Store.CreateCloudNodeLink(c.Context(), *link); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "instance was provisioned but its node link could not be saved; instance ID: "+instance.ID)
			}
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": instance, "link": link})
	})

	cl.Delete("/instances/:provider/:id", mutationLimiter, requireRole("admin"), requireAdminScope("cloud.write"), func(c *fiber.Ctx) error {
		provider := cloud.ProviderKind(c.Params("provider"))
		instanceID := c.Params("id")
		if err := svc.DeprovisionNode(c.Context(), provider, instanceID); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "cloud provider could not terminate instance: "+err.Error())
		}
		if cfg.Store != nil {
			if err := cfg.Store.DeleteCloudNodeLink(c.Context(), string(provider), instanceID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "instance was terminated but its node link could not be removed")
			}
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
